package api

import "context"

type Folder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type FolderItem struct {
	Type   string  `json:"type"`
	Folder *Folder `json:"folder,omitempty"`
	Design *Design `json:"design,omitempty"`
}

// WalkFolders walks from "root" and "uploads" special folder IDs and emits
// every folder it encounters via visit.
func (c *Client) WalkFolders(ctx context.Context, visit func(folder Folder, parentID string) error) error {
	for _, root := range []string{"root", "uploads"} {
		if err := c.walkFolder(ctx, root, "", visit); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) walkFolder(ctx context.Context, id, parentID string, visit func(Folder, string) error) error {
	return Paginate[FolderItem](ctx, c, "/folders/"+id+"/items", func(item FolderItem) error {
		if item.Type != "folder" || item.Folder == nil {
			return nil
		}
		if err := visit(*item.Folder, parentID); err != nil {
			return err
		}
		return c.walkFolder(ctx, item.Folder.ID, item.Folder.ID, visit)
	})
}
