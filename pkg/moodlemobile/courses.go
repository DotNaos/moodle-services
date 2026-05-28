package moodlemobile

type Course struct {
	ID        int    `json:"id"`
	FullName  string `json:"fullName"`
	ShortName string `json:"shortName"`
	Category  string `json:"category,omitempty"`
	HeroImage string `json:"heroImage,omitempty"`
	ViewURL   string `json:"viewUrl,omitempty"`
}

type mobileCourse struct {
	ID          int    `json:"id"`
	FullName    string `json:"fullname"`
	ShortName   string `json:"shortname"`
	Visible     int    `json:"visible"`
	CategoryID  int    `json:"category"`
	CourseImage string `json:"courseimage"`
}
