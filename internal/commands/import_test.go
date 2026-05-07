package commands

import "testing"

// TestImportRoutingTable_Coverage spot-checks that every extension in
// spec §8 maps to the correct endpoint. Driven by the table; the keys
// here are the spec rows.
func TestImportRoutingTable_Coverage(t *testing.T) {
	cases := []struct {
		ext    string
		target string
	}{
		// Images → asset uploads.
		{".png", "asset"}, {".jpg", "asset"}, {".jpeg", "asset"},
		{".gif", "asset"}, {".heic", "asset"}, {".tiff", "asset"}, {".webp", "asset"},
		// Videos → asset uploads.
		{".mp4", "asset"}, {".mov", "asset"}, {".webm", "asset"},
		// Documents → /imports.
		{".pdf", "import"}, {".pptx", "import"}, {".docx", "import"},
		{".xlsx", "import"}, {".key", "import"}, {".pages", "import"},
		{".numbers", "import"}, {".ai", "import"}, {".psd", "import"},
		{".afdesign", "import"}, {".odp", "import"},
	}
	for _, tc := range cases {
		got, ok := importRoutingTable[tc.ext]
		if !ok {
			t.Errorf("%s missing from routing table", tc.ext)
			continue
		}
		if got.target != tc.target {
			t.Errorf("%s: target=%q want %q", tc.ext, got.target, tc.target)
		}
	}
}

// TestImportRoutingTable_PdfMime verifies the PDF mime type — this is the
// extension that the spec calls out for explicit mime sniffing.
func TestImportRoutingTable_PdfMime(t *testing.T) {
	r, ok := importRoutingTable[".pdf"]
	if !ok || r.mimeType != "application/pdf" {
		t.Errorf("pdf route: %+v", r)
	}
}

// TestImportRoutingTable_UnknownExt verifies an unknown extension is not
// in the table — the command-level handler turns that into the
// import_unsupported_format error.
func TestImportRoutingTable_UnknownExt(t *testing.T) {
	if _, ok := importRoutingTable[".xyz"]; ok {
		t.Error("xyz should not be in the routing table")
	}
}
