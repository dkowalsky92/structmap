package models2

type UserDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Age    int    `json:"age"`
	Height int    `json:"height"`
}
