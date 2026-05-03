package moodle

type Category struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	IDNumber string `json:"idNumber,omitempty"`
	ParentID int    `json:"parentId,omitempty"`
	Path     string `json:"path,omitempty"`
	Depth    int    `json:"depth,omitempty"`
}

func CategoryNameByID(categories []Category) map[int]string {
	names := make(map[int]string, len(categories))
	for _, category := range categories {
		if category.ID != 0 && category.Name != "" {
			names[category.ID] = category.Name
		}
	}
	return names
}
