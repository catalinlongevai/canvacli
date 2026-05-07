package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/spf13/cobra"
)

// importRoute describes how a given extension is dispatched: either as a
// document import (POST /v1/imports) or a media asset upload
// (POST /v1/asset-uploads). Mirrors spec §8.
type importRoute struct {
	target   string // "import" | "asset"
	mimeType string // populated for the import route; "" lets Canva sniff
}

// importRoutingTable mirrors the §8 Import Routing table verbatim.
var importRoutingTable = map[string]importRoute{
	// Images → asset upload (PNG/JPEG are NOT accepted by /imports).
	".png":  {target: "asset"},
	".jpg":  {target: "asset"},
	".jpeg": {target: "asset"},
	".gif":  {target: "asset"},
	".heic": {target: "asset"},
	".tiff": {target: "asset"},
	".webp": {target: "asset"},

	// Videos → asset upload.
	".mp4":  {target: "asset"},
	".mov":  {target: "asset"},
	".m4v":  {target: "asset"},
	".mkv":  {target: "asset"},
	".mpeg": {target: "asset"},
	".webm": {target: "asset"},

	// Documents → /imports.
	".pdf":      {target: "import", mimeType: "application/pdf"},
	".pptx":     {target: "import", mimeType: "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
	".ppt":      {target: "import", mimeType: "application/vnd.ms-powerpoint"},
	".docx":     {target: "import", mimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	".doc":      {target: "import", mimeType: "application/msword"},
	".xlsx":     {target: "import", mimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	".xls":      {target: "import", mimeType: "application/vnd.ms-excel"},
	".key":      {target: "import"},
	".pages":    {target: "import"},
	".numbers":  {target: "import"},
	".ai":       {target: "import"},
	".psd":      {target: "import"},
	".afdesign": {target: "import"},
	".afphoto":  {target: "import"},
	".afpub":    {target: "import"},
	".af":       {target: "import"},
	".odp":      {target: "import"},
	".odt":      {target: "import"},
	".ods":      {target: "import"},
	".odg":      {target: "import"},
}

// NewImport builds the `canva import <file>` command with extension-based
// routing per spec §8: images/videos → asset-uploads, documents → imports,
// anything else → stable error code import_unsupported_format (exit 5).
func NewImport() *cobra.Command {
	var flagMime string
	var flagListFormats bool
	var flagIDOnly bool
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import a local PDF/PPTX/DOCX/etc as a Canva design (or upload images as assets)",
		Long: `Import a local file. Routing is decided by extension (lowercased):

  - PDF, PPTX, DOCX, XLSX, Keynote/Pages/Numbers, AI/PSD, Affinity, OpenOffice
    → POST /v1/imports (creates a new Canva design).
  - PNG/JPEG/GIF/HEIC/TIFF/WEBP/MP4/MOV/etc.
    → POST /v1/asset-uploads (imports are for documents; the file is added
    to the asset library and the asset id is returned instead).

Pass --mime-type to override the imports endpoint's mime sniffing.
Use --list-formats to print the routing table.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if flagListFormats {
				return nil
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagListFormats {
				return printSupportedFormats(os.Stdout)
			}

			path := args[0]
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("open %s: %w", path, err)
			}
			defer file.Close()

			ext := strings.ToLower(filepath.Ext(path))
			route, ok := importRoutingTable[ext]
			if !ok {
				return &api.APIError{
					Code:    "import_unsupported_format",
					Message: fmt.Sprintf("extension %q is not a supported import format — run `canva import --list-formats` to see the full list", ext),
				}
			}

			ctx := cmd.Context()
			cl, err := loadClient(ctx)
			if err != nil {
				return err
			}
			ch, err := loadCache()
			if err != nil {
				return err
			}
			defer ch.Close()

			name := filepath.Base(path)

			if route.target == "asset" {
				asset, err := cl.UploadAsset(ctx, name, file)
				if err != nil {
					return err
				}
				if err := upsertAssetIntoCache(ch, asset); err != nil {
					fmt.Fprintf(os.Stderr, "warn: cache upsert failed: %v\n", err)
				}
				// One-line note that imports are for documents (per spec §8 routing copy).
				fmt.Fprintln(os.Stderr, "note: imports are for documents; uploaded as asset instead.")
				if flagIDOnly {
					fmt.Println(asset.ID)
					return nil
				}
				out := map[string]any{
					"kind":     "asset",
					"asset_id": asset.ID,
					"id":       asset.ID,
					"name":     asset.Name,
					"type":     asset.Type,
				}
				if asset.Thumbnail != nil && asset.Thumbnail.URL != "" {
					out["thumbnail_url"] = asset.Thumbnail.URL
				}
				return output.EmitJSON(os.Stdout, out)
			}

			// Document → /imports.
			mime := flagMime
			if mime == "" {
				mime = route.mimeType
			}
			res, err := cl.ImportFile(ctx, name, mime, file)
			if err != nil {
				return err
			}
			if len(res.Result.Designs) == 0 {
				return fmt.Errorf("import returned no designs")
			}

			// Cache each new design so subsequent commands (export, pages,
			// resolver-by-name) can find them.
			now := time.Now().Unix()
			for _, d := range res.Result.Designs {
				raw, _ := json.Marshal(d)
				thumb := ""
				if d.Thumbnail != nil {
					thumb = d.Thumbnail.URL
				}
				_ = ch.UpsertDesign(cache.Design{
					ID:           d.ID,
					Title:        d.Title,
					UpdatedAt:    d.UpdatedAt,
					FetchedAt:    now,
					ThumbnailURL: thumb,
					RawJSON:      string(raw),
				})
			}

			if flagIDOnly {
				// One id per line — typically one for single-PDF imports.
				for _, d := range res.Result.Designs {
					fmt.Println(d.ID)
				}
				return nil
			}

			designs := make([]map[string]any, 0, len(res.Result.Designs))
			for _, d := range res.Result.Designs {
				row := map[string]any{
					"id":         d.ID,
					"title":      d.Title,
					"page_count": d.PageCount,
				}
				if d.URLs != nil {
					if d.URLs.EditURL != "" {
						row["edit_url"] = d.URLs.EditURL
					}
					if d.URLs.ViewURL != "" {
						row["view_url"] = d.URLs.ViewURL
					}
				}
				designs = append(designs, row)
			}
			return output.EmitJSON(os.Stdout, map[string]any{
				"kind":    "import",
				"designs": designs,
			})
		},
	}
	cmd.Flags().StringVar(&flagMime, "mime-type", "", "override mime type for the imports endpoint")
	cmd.Flags().BoolVar(&flagListFormats, "list-formats", false, "print supported import formats and the routing table, then exit")
	cmd.Flags().BoolVar(&flagIDOnly, "id-only", false, "print only the bare design or asset id(s) to stdout")
	return cmd
}

// printSupportedFormats writes the full routing table to w in a stable order.
func printSupportedFormats(w io.Writer) error {
	type row struct {
		Ext    string `json:"ext"`
		Target string `json:"target"`
		Mime   string `json:"mime,omitempty"`
	}
	rows := make([]row, 0, len(importRoutingTable))
	for ext, route := range importRoutingTable {
		rows = append(rows, row{Ext: ext, Target: route.target, Mime: route.mimeType})
	}
	return output.EmitJSON(w, map[string]any{
		"routes":      rows,
		"description": "ext → /v1/imports for documents, /v1/asset-uploads for images and videos. PNG/JPEG are NOT supported by /v1/imports.",
	})
}
