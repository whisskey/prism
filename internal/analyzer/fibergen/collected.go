// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

// Route is a discovered HTTP route (Pass 1).
type Route struct {
	Method       string
	Path         string
	Handler      string
	HandlerRef   string
	Middlewares  []string
	Prefix       string
	SourceFile   string
	SourceLine   int
	ReceiverType string
}

// HandlerDetail holds analyzed handler semantics (Pass 2).
type HandlerDetail struct {
	Name            string
	ResponseType    string
	RequestBody     string
	ResponseSchemas map[string]string // status code → schema (helper wrappers)
	QueryParams     []Param
	PathParams      []Param
	SourceFile      string
	SourceLine      int
}

// Param is a path or query parameter inferred from handler body.
type Param struct {
	Name     string
	GoType   string
	Required bool
}

// Schema is an OpenAPI 3 schema (Pass 3).
type Schema struct {
	Name       string
	Type       string              `yaml:"type"`
	Format     string              `yaml:"format,omitempty"`
	Properties map[string]Property `yaml:"properties,omitempty"`
	Required   []string            `yaml:"required,omitempty"`
	Items      *Schema             `yaml:"items,omitempty"`
	Ref        string              `yaml:"$ref,omitempty"`
}

// Property is one field on an OpenAPI object schema.
type Property struct {
	Type      string   `yaml:"type,omitempty"`
	Format    string   `yaml:"format,omitempty"`
	Ref       string   `yaml:"$ref,omitempty"`
	Items     *Schema  `yaml:"items,omitempty"`
	Minimum   *float64 `yaml:"minimum,omitempty"`
	Maximum   *float64 `yaml:"maximum,omitempty"`
	MinLength *int     `yaml:"minLength,omitempty"`
	MaxLength *int     `yaml:"maxLength,omitempty"`
	Enum      []string `yaml:"enum,omitempty"`
}

// SecurityRequirement references a security scheme on a route (Pass 4).
type SecurityRequirement struct {
	Scheme string
	Scopes []string
}

// SecurityScheme is an OpenAPI 3 security scheme definition (Pass 4).
type SecurityScheme struct {
	Type   string `yaml:"type"`
	Scheme string `yaml:"scheme,omitempty"`
	In     string `yaml:"in,omitempty"`
	Name   string `yaml:"name,omitempty"`
}
