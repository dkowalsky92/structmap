package models2

type DescriptionDTO struct {
	Hobbies   []string `json:"hobby"`
	Interests []string `json:"interests"`
}

type UserDTO struct {
	DescriptionDTO
	ID                   string         `json:"id"`
	Name                 *string        `json:"name"`
	LastName             string         `json:"last_name"`
	Age                  int            `json:"age"`
	Height               *int           `structmap:"user_dto_height"`
	About                *string        `json:"about"`
	AdditionalProperties map[string]any `json:"additional_properties"`
}
