package models1

import "github.com/google/uuid"

type Description struct {
	Hobbies   []string `json:"hobby"`
	Interests []string `json:"interests"`
}

type User struct {
	Description
	ID                   uuid.UUID              `json:"id"`
	FirstName            string                 `json:"first_name"`
	Age                  int                    `json:"age"`
	UserHeight           int                    `structmap:"user_height"`
	AdditionalProperties map[string]interface{} `json:"additional_properties"`
}
