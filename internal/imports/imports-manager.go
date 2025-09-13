package imports

import (
	"fmt"
	"strings"
)

type ImportManager struct {
	imports      map[string]string
	aliasCounter int
}

func NewImportManager() *ImportManager {
	return &ImportManager{
		imports:      make(map[string]string),
		aliasCounter: 1,
	}
}

func (im *ImportManager) AddImport(importPath string) {
	if !strings.Contains(importPath, "/") {
		return
	}
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return
	}
	importPath = strings.Trim(importPath, "\"")
	if _, exists := im.imports[importPath]; exists {
		return
	}

	alias := fmt.Sprintf("ref%d", im.aliasCounter)
	im.aliasCounter++

	im.imports[importPath] = alias
}

func (im *ImportManager) GetImportAlias(importPath string) string {
	importPath = strings.Trim(importPath, "\"")
	return im.imports[importPath]
}

func (im *ImportManager) RenderImports(pattern string) string {
	if len(im.imports) == 0 {
		return ""
	}

	var imports []string
	for importPath, alias := range im.imports {
		if strings.Contains(pattern, alias+".") {
			imports = append(imports, fmt.Sprintf("\t%s \"%s\"", alias, importPath))
		}
	}

	return fmt.Sprintf("import (\n%s\n)", strings.Join(imports, "\n"))
}
