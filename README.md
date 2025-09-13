# structmap

Generate strongly-typed mapper functions between Go structs from a small YAML spec. You describe where to map from and to, optionally add conversions and custom field wiring, and structmap generates readable Go code that you can commit.

- **Inputs**: `config.yaml` (mappings) + `conversions.yaml` (type conversions)
- **Output**: a Go file with pure functions: `Map<FromType>To<ToType>(src <FromType>, ...) <ToType>`

## Features
- **Zero runtime dependency**: emits plain Go; no reflection at runtime
- **Name / tag matching**: fields are matched by name, then by tag (default: `json`)
- **Custom field mapping**: override specific fields via `custom_field_mappings`
- **Additional function args**: inject values not present on source via `func_additional_args`
- **Error handling**: conversions can return an error via `error: true`
- **Typed conversions**: reusable templated conversions with per-conversion imports
- **Automatic imports**: required imports are discovered and aliased deterministically
- **Type alias traversal**: follows type aliases across packages to find the underlying struct
- **Embedded field flattening**: anonymous/embedded struct fields are recursively inlined

## Install
Install as a Go tool (Go 1.24+):
```bash
go get -tool github.com/dkowalsky92/structmap/cmd/structmap@latest
```

The module uses `golang.org/x/tools/go/packages` to load source packages; run the tool within your module so import paths resolve.

## Quickstart
1) Define your source and destination structs (can live in different packages).
2) Write a `conversions.yaml` for common type conversions (optional).
3) Write a `config.yaml` mapping the structs and any customizations.
4) Create a `//go:generate` directive to run the tool.
5) Run `go generate`

## Examples

### Simple

`examples/simple/models1/model1.go`
```go
package models1

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
	Height int  `json:"height"`
}
```

`examples/simple/models2/model2.go`
```go
package models2

type UserDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
	Height int  `json:"height"`
}
```

`examples/simple/mapping/config.yaml`
```yaml
out_package_name: main
mappings:
  - from:
      type: "{{ .Import0 }}.User"
      imports:
        - "github.com/dkowalsky92/structmap/examples/simple/models1"
    to:
      type: "{{ .Import0 }}.UserDTO"
      imports:
        - "github.com/dkowalsky92/structmap/examples/simple/models2"
```

Generated (`examples/simple/structmap.gen.go`):
```go
// MapUserToUserDTO copies User → UserDTO
func MapUserToUserDTO(src ref1.User) (dst ref2.UserDTO) {
	dst.ID = src.ID
	dst.Name = src.Name
	dst.Age = src.Age
	dst.Height = src.Height
	return
}
```

### Complex

`examples/complex/models1/model1.go`
```go
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
```

`examples/complex/models2/model2.go`
```go
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
```

`examples/complex/mapping/config.yaml`
```yaml
out_package_name: mapping
out_file_name: mapping.gen.go
out_file_path: .generated
debug: true
mappings:
  - from:
      type: "{{ .Import0 }}.User"
      imports:
        - "github.com/dkowalsky92/structmap/examples/complex/models1"
    to:
      type: "{{ .Import0 }}.UserDTO"
      imports:
        - "github.com/dkowalsky92/structmap/examples/complex/models2"
    func_additional_args:
      - name: about
        type: "*string"
        dest_field: "About"
    custom_field_mappings:
      - source_field: "FirstName"
        dest_field: "Name"
      - source_tag: "user_height"
        dest_tag: "user_dto_height"
        tag: "structmap"

  - from:
      type: "{{ .Import0 }}.UserDTO"
      imports:
        - "github.com/dkowalsky92/structmap/examples/complex/models2"
    to:
      type: "{{ .Import0 }}.User"
      imports:
        - "github.com/dkowalsky92/structmap/examples/complex/models1"
    custom_field_mappings:
      - source_field: "Name"
        dest_field: "FirstName"
      - source_tag: "user_dto_height"
        dest_tag: "user_height"
        tag: "structmap"
```

`examples/complex/mapping/conversions.yaml`
```yaml
conversions:
  - source_type: int
    dest_type: "*int"
    conversion:
      tmpl: "{{ .Dest }} = &{{ .Source }}"
    reverse_conversion:
      tmpl: "{{ .Dest }} = *{{ .Source }}"

  - source_type: string
    dest_type: "*string"
    conversion:
      tmpl: "{{ .Dest }} = &{{ .Source }}"
    reverse_conversion:
      tmpl: "{{ .Dest }} = *{{ .Source }}"

  - source_type: "{{ .Import0 }}.UUID"
    dest_type: string
    conversion:
      tmpl: "{{ .Dest }} = {{ .Source }}.String()"
    reverse_conversion:
      error: true
      tmpl: "{{ .Dest }}, {{ .Error }} = {{ .Import0 }}.Parse({{ .Source }})"
    imports:
      - "github.com/google/uuid"
```

Generated (`examples/complex/.generated/mapping.gen.go`):
```go
// MapUserToUserDTO copies User → UserDTO
func MapUserToUserDTO(src ref2.User, about *string) (dst ref3.UserDTO) {
	dst.Hobbies = src.Hobbies
	dst.Interests = src.Interests
	dst.ID = src.ID.String()
	dst.Name = &src.FirstName
	// no matching source found for field: LastName, consider adding an additional arg or aligning the fields
	dst.Age = src.Age
	dst.Height = &src.UserHeight
	dst.About = about
	dst.AdditionalProperties = src.AdditionalProperties
	return
}

// MapUserDTOToUser copies UserDTO → User
func MapUserDTOToUser(src ref3.UserDTO) (dst ref2.User, err error) {
	dst.Hobbies = src.Hobbies
	dst.Interests = src.Interests
    dst.ID, err = ref1.Parse(src.ID)
	dst.FirstName = *src.Name
	dst.Age = src.Age
	dst.UserHeight = *src.Height
	dst.AdditionalProperties = src.AdditionalProperties
	return
}
```

## Configuration schema
`config.yaml`
```yaml
out_package_name: string          # required, package name for the generated file
out_file_name: string             # optional, filename for the generated file (default: "structmap.gen.go")
out_file_path: string             # optional, directory path for the generated file (default: ".")
debug: bool                       # optional, whether to print debug information (default: false)
tag: string                       # optional, tag key (default: "json")
mappings:
  - from:                         # required, source struct definition
      type: string                # required, struct type template (see Type Templates)
      imports:                    # optional, imports used by the type template
        - string                 

    to:                           # required, destination struct definition
      type: string                # required, struct type template (see Type Templates)
      imports:                    # optional, imports used by the type template
        - string

    func_name: string             # optional, function name (default: "Map<FromType>To<ToType>")

    func_additional_args:         # optional, extra function parameters
      - name: string              # required, name of the argument
        dest_field: string        # required, which destination field this argument feeds
        type: string              # required, templated type (see Type Templates)
        imports:                  # optional, imports used by the type template
          - string

    custom_field_mappings:        # optional, either name-based or tag-based override
      - source_field: string      # optional, name-based override (source_field + dest_field)
        dest_field: string        
        source_tag: string        # optional, tag-based override (dest_tag + source_tag)
        dest_tag: string          
        tag: string               # optional, tag key (default: "json")

    custom_conversions:           # optional, conversions only for this mapping
      - source_type: string       # required, templated type (see Type Templates)
        dest_type: string         # required, templated type (see Type Templates)
        conversion: 
          tmpl: string            # required, template applied used for assignment (see Conversions)
          error: bool             # optional, whether the conversion can return an error
        reverse_conversion:
          tmpl: string            # optional, template applied used for reverse assignment (see Conversions)
          error: bool             # optional, whether the conversion can return an error
        imports:                  # optional, imports used by this conversion template
          - string
```

`conversions.yaml`
```yaml
conversions:
  - source_type: string           # required, templated type (see Type Templates)
    dest_type: string             # required, templated type (see Type Templates)
    conversion: 
      tmpl: string                # required, template applied used for assignment (see Conversions)
      error: bool                 # optional, whether the conversion can return an error
    reverse_conversion:
      tmpl: string                # optional, template applied used for reverse assignment (see Conversions)
      error: bool                 # optional, whether the conversion can return an error
    imports:                      # optional, imports used by this conversion
      - string
```

### Type Templates
Anywhere a type is specified (`from`/`to` types, custom_conversions `source_type`, `dest_type`, additional arg `type`), you can use placeholders referencing per-item imports:

Example:
```yaml
// config.yaml
...
custom_conversions:
  - source_type: "{{ .Import0 }}.UUID"
    dest_type: "string"
    imports: 
      - "github.com/google/uuid"
...
```

The tool assigns deterministic aliases (`ref1`, `ref2`, ...) and renders types and expressions with those aliases. Only imports actually referenced in the generated code are emitted.

### Conversions
Conversions are small Go text/templates:
- `{{ .Source }}` is the source expression
- `{{ .Dest }}` is the destination expression
- `{{ .Error }}` is the error expression
- `{{ .ImportN }}` are the per-conversion imports, N is the index of the import

Examples:
- `int` → `*int`: `{{ .Dest }} = &{{ .Source }}`
- `*time.Time` → `time.Time`: `{{ .Dest }} = {{ .Source }}.UTC()` with `imports: ["time"]`
- Optional reverse conversions are supported via `reverse_conversion` when mapping in the opposite direction, `{{ .Source }}` and `{{ .Dest }}` are swapped in this case.

### Field matching rules
- First applies `custom_field_mappings` overrides (supports name and tag based overrides)
- Second tries exact field name match
- Then tries tag match using `tag` (default: `json`)
- If nothing matches, a comment is left in the generated code for that field
\- Embedded fields are flattened recursively and participate in matching. If multiple source fields collide by name or tag, the later one wins; use `custom_field_mappings` to disambiguate explicitly.

### Function signature
If `func_name` is omitted, generator emits:
```
Map<FromType>To<ToType>(src <FromType>, [additional args...]) <ToType>
```
Additional args are included in the order defined in `func_additional_args` and are used to fill their `dest_field`, with conversions applied when needed.

### Matching configuration
- `tag` (per-mapping): select which struct tag key to use for default tag-based matching; default is `json`.
- `custom_field_mappings` supports:
  - name-based: `source_field` + `dest_field`
  - tag-based: `source_tag` + `dest_tag`, optional `tag` overrides the tag name for this override only.

## Constraints and notes
- Designed for struct-to-struct mapping; interface/primitive top-level types are not supported
- The tool loads packages by import path; run within a proper Go module so imports resolve
- Imports are emitted only if actually used in the generated body
- Generated files start with `// Code generated by structmap; DO NOT EDIT.`