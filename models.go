package main

import (
    "gorm.io/gorm"
)

type Feature struct {
    gorm.Model `json:"-"`
    MergeId        string `json:"merge_id" gorm:"index:unique"`
    Name  string `json:"name" validate:"omitempty,max=32"`
    Description string `json:"description" validate:"omitempty,max=380"`
    EmbeddedLink       string    `json:"embedded_link" validate:"omitempty,url"`
    Color     string `json:"color" validate:"omitempty,hexcolor"`
	LinkToVR	string `json:"link_to_vr" validate:"omitempty,url"`

}
