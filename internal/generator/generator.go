package generator

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"html/template"
	"log"
	"reflect"
	"strings"

	"go/printer"

	"github.com/dkowalsky92/structmap/internal/imports"
	"github.com/dkowalsky92/structmap/internal/packages"
)

type Conversions struct {
	Conversions []Conversion `yaml:"conversions"`
}

type Config struct {
	OutPackageName string    `yaml:"out_package_name"`
	OutFileName    string    `yaml:"out_file_name,omitempty"`
	OutFilePath    string    `yaml:"out_file_path,omitempty"`
	Mappings       []Mapping `yaml:"mappings"`
	Debug          bool      `yaml:"debug,omitempty"`
}

type Mapping struct {
	From                StructDefinition     `yaml:"from"`
	To                  StructDefinition     `yaml:"to"`
	FuncName            string               `yaml:"func_name,omitempty"`
	FuncAdditionalArgs  []AdditionalArg      `yaml:"func_additional_args,omitempty"`
	CustomFieldMappings []CustomFieldMapping `yaml:"custom_field_mappings,omitempty"`
	CustomConversions   []Conversion         `yaml:"custom_conversions,omitempty"`
	Tag                 string               `yaml:"tag,omitempty"`
}

type CustomFieldMapping struct {
	SourceField string `yaml:"source_field,omitempty"`
	DestField   string `yaml:"dest_field,omitempty"`
	SourceTag   string `yaml:"source_tag,omitempty"`
	DestTag     string `yaml:"dest_tag,omitempty"`
	Tag         string `yaml:"tag,omitempty"`
}

type AdditionalArg struct {
	Name                    string `yaml:"name"`
	DestField               string `yaml:"dest_field"`
	TypeWithImportsTemplate `yaml:",inline"`
}

func (a *AdditionalArg) RenderParameter(importManager *imports.ImportManager) string {
	renderedType := a.ExecuteTemplate(importManager)
	return fmt.Sprintf("%s %s", a.Name, renderedType)
}

type Conversion struct {
	SourceType        string             `yaml:"source_type"`
	DestType          string             `yaml:"dest_type"`
	Conversion        ConversionTemplate `yaml:"conversion"`
	ReverseConversion ConversionTemplate `yaml:"reverse_conversion,omitempty"`
	Imports           []string           `yaml:"imports"`
}

type ConversionTemplate struct {
	Tmpl  string `yaml:"tmpl"`
	Error bool   `yaml:"error,omitempty"`
}

func (c *Conversion) GetSourceTypeWithImportsTemplate() TypeWithImportsTemplate {
	return NewTypeWithImportsTemplate(c.SourceType, c.Imports)
}

func (c *Conversion) GetDestTypeWithImportsTemplate() TypeWithImportsTemplate {
	return NewTypeWithImportsTemplate(c.DestType, c.Imports)
}

func (c *Conversion) ExecuteConversionTemplate(sourceVar string, destVar string, errorVar string, importManager *imports.ImportManager) (string, bool) {
	return c.executeTemplate(c.Conversion.Tmpl, c.Conversion.Error, sourceVar, destVar, errorVar, importManager, "conversion")
}

func (c *Conversion) ExecuteReverseConversionTemplate(sourceVar string, destVar string, errorVar string, importManager *imports.ImportManager) (string, bool) {
	if c.ReverseConversion.Tmpl == "" {
		return fmt.Sprintf("%s = %s", destVar, sourceVar), false
	}
	return c.executeTemplate(c.ReverseConversion.Tmpl, c.ReverseConversion.Error, sourceVar, destVar, errorVar, importManager, "reverse_conversion")
}

func (c *Conversion) executeTemplate(tmplStr string, hasError bool, sourceVar, destVar, errorVar string, importManager *imports.ImportManager, tmplName string) (string, bool) {
	var buf strings.Builder
	tmpl, err := template.New(tmplName).Parse(tmplStr)
	if err != nil {
		panic(err)
	}
	data := make(map[string]string)
	for idx, imp := range c.Imports {
		data[fmt.Sprintf("Import%d", idx)] = importManager.GetImportAlias(imp)
	}
	data["Source"] = sourceVar
	data["Dest"] = destVar
	data["Error"] = errorVar
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(err)
	}
	return buf.String(), hasError
}

type StructDefinition struct {
	TypeWithImportsTemplate `yaml:",inline"`
}

type FieldDefinition struct {
	Name string
	Tag  string
	TypeWithImportsTemplate
}

func NewFieldDefinition(name, typeStr, tag string, importInfos []ImportInfo) FieldDefinition {
	typeTemplate := typeStr
	imports := make([]string, len(importInfos))
	for idx, importInfo := range importInfos {
		old := importInfo.PkgName
		if importInfo.Alias != nil {
			old = *importInfo.Alias
		}
		typeTemplate = strings.ReplaceAll(typeTemplate, old, fmt.Sprintf("{{ .Import%d }}", idx))
		imports[idx] = importInfo.Path
	}
	return FieldDefinition{
		Name:                    name,
		Tag:                     tag,
		TypeWithImportsTemplate: NewTypeWithImportsTemplate(typeTemplate, imports),
	}
}

type ImportInfo struct {
	Alias   *string
	PkgName string
	Path    string
}

func NewImportInfo(alias *string, pkgName string, path string) ImportInfo {
	return ImportInfo{
		Alias:   alias,
		PkgName: pkgName,
		Path:    path,
	}
}

type TypeWithImportsTemplate struct {
	TypeTemplate string   `yaml:"type"`
	Imports      []string `yaml:"imports"`
}

func NewTypeWithImportsTemplate(typeStr string, imports []string) TypeWithImportsTemplate {
	return TypeWithImportsTemplate{
		TypeTemplate: typeStr,
		Imports:      imports,
	}
}

func (t TypeWithImportsTemplate) ExecuteTemplate(importManager *imports.ImportManager) string {
	var buf strings.Builder
	tmpl, err := template.New("type").Parse(t.TypeTemplate)
	if err != nil {
		panic(fmt.Sprintf("failed to parse type template: %v", err))
	}

	data := make(map[string]string)
	for idx, imp := range t.Imports {
		data[fmt.Sprintf("Import%d", idx)] = importManager.GetImportAlias(imp)
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("failed to execute type template: %v", err))
	}
	return buf.String()
}

func (t TypeWithImportsTemplate) GetUnaliasedType() string {
	result := t.TypeTemplate
	for i := 0; i < len(t.Imports); i++ {
		pattern := fmt.Sprintf("{{ .Import%d }}.", i)
		result = strings.ReplaceAll(result, pattern, "")
	}
	return result
}

func (t TypeWithImportsTemplate) Equals(other TypeWithImportsTemplate, importManager *imports.ImportManager) bool {
	renderedT := t.ExecuteTemplate(importManager)
	renderedOther := other.ExecuteTemplate(importManager)
	return renderedT == renderedOther
}

type Generator struct {
	importManager   *imports.ImportManager
	packageManager  *packages.PackageManager
	typeToFieldsMap map[string][]FieldDefinition
	conversions     Conversions
	config          Config
}

func NewGenerator(config Config, conversions Conversions) *Generator {
	return &Generator{
		importManager:   imports.NewImportManager(),
		packageManager:  packages.NewPackageManager(),
		typeToFieldsMap: make(map[string][]FieldDefinition),
		conversions:     conversions,
		config:          config,
	}
}

func (g *Generator) AddFields(typeName string, fields []FieldDefinition) {
	g.typeToFieldsMap[typeName] = fields
}

func (g *Generator) GetFields(typeName string) ([]FieldDefinition, bool) {
	fields, exists := g.typeToFieldsMap[typeName]
	return fields, exists
}

func (g *Generator) Generate() (string, error) {
	var funcs []string

	for _, conversion := range g.conversions.Conversions {
		for _, imp := range conversion.Imports {
			g.importManager.AddImport(imp)
		}
	}

	for _, mapping := range g.config.Mappings {
		for _, customConversion := range mapping.CustomConversions {
			for _, imp := range customConversion.Imports {
				g.importManager.AddImport(imp)
			}
		}

		for _, additionalArg := range mapping.FuncAdditionalArgs {
			for _, imp := range additionalArg.Imports {
				g.importManager.AddImport(imp)
			}
		}

		for _, imp := range mapping.From.Imports {
			g.importManager.AddImport(imp)
		}
		for _, imp := range mapping.To.Imports {
			g.importManager.AddImport(imp)
		}

		fromPkgPath := ""
		if len(mapping.From.Imports) > 0 {
			fromPkgPath = mapping.From.Imports[0]
		}
		fromFields, err := g.extractFieldsFromPackage(fromPkgPath, mapping.From.GetUnaliasedType())
		if err != nil {
			return "", fmt.Errorf("failed to extract fields from %s: %w", mapping.From.ExecuteTemplate(g.importManager), err)
		}
		for _, field := range fromFields {
			for _, imp := range field.Imports {
				g.importManager.AddImport(imp)
			}
		}

		toPkgPath := ""
		if len(mapping.To.Imports) > 0 {
			toPkgPath = mapping.To.Imports[0]
		}
		toFields, err := g.extractFieldsFromPackage(toPkgPath, mapping.To.GetUnaliasedType())
		if err != nil {
			return "", fmt.Errorf("failed to extract fields to %s: %w", mapping.To.ExecuteTemplate(g.importManager), err)
		}
		for _, field := range toFields {
			for _, imp := range field.Imports {
				g.importManager.AddImport(imp)
			}
		}

		g.AddFields(mapping.From.TypeTemplate, fromFields)
		g.AddFields(mapping.To.TypeTemplate, toFields)

		funcCode, err := g.generateFunction(mapping)
		if err != nil {
			return "", fmt.Errorf("failed to generate function: %w", err)
		}
		funcs = append(funcs, funcCode)
	}

	funcCode := strings.Join(funcs, "\n\n")
	importCode := g.importManager.RenderImports(funcCode)

	code := fmt.Sprintf(`// Code generated by structmap; DO NOT EDIT.
package %s

%s

%s
`, g.config.OutPackageName, importCode, funcCode)

	return code, nil
}

func (g *Generator) extractFieldsFromPackage(pkgPath string, typeName string) ([]FieldDefinition, error) {
	structDef, structPkgPath, err := g.findStructDefinition(pkgPath, typeName)
	if err != nil {
		return nil, err
	}
	var fields []FieldDefinition
	for _, fld := range structDef.Fields.List {
		var buf strings.Builder
		fset := token.NewFileSet()
		printer.Fprint(&buf, fset, fld.Type)
		typ := buf.String()

		importInfos, err := g.findImportSpecsForExpression(fld.Type, structPkgPath)
		if err != nil {
			return nil, fmt.Errorf("failed to find import specs for expression: %w", err)
		}
		tag := ""
		if fld.Tag != nil {
			tag = strings.Trim(fld.Tag.Value, "`")
		}
		if len(fld.Names) == 0 {
			embeddedFields, err := g.expandEmbeddedFields(fld, structPkgPath)
			if err != nil {
				return nil, fmt.Errorf("failed to expand embedded field: %w", err)
			}
			fields = append(fields, embeddedFields...)
			continue
		}
		for _, name := range fld.Names {
			fields = append(fields, NewFieldDefinition(name.Name, typ, tag, importInfos))
		}
	}
	return fields, nil
}

func (g *Generator) generateFunction(mapping Mapping) (string, error) {
	sourceFields, ok1 := g.GetFields(mapping.From.TypeTemplate)
	destFields, ok2 := g.GetFields(mapping.To.TypeTemplate)
	if !ok1 || !ok2 {
		return "", fmt.Errorf("structs not found: %s, %s", mapping.From.TypeTemplate, mapping.To.TypeTemplate)
	}
	if g.config.Debug {
		sourceFieldsJSON, err := json.MarshalIndent(sourceFields, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal source fields: %w", err)
		}
		destFieldsJSON, err := json.MarshalIndent(destFields, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal dest fields: %w", err)
		}
		log.Printf("Source fields:\n%s", string(sourceFieldsJSON))
		log.Printf("Dest fields:\n%s", string(destFieldsJSON))
	}
	byName := map[string]FieldDefinition{}
	tag := mapping.Tag
	if tag == "" {
		tag = "json"
	}
	byTag := map[string]FieldDefinition{}
	for _, sourceField := range sourceFields {
		byName[sourceField.Name] = sourceField
		if tv := tagValue(sourceField.Tag, tag); tv != "" {
			byTag[tv] = sourceField
		}
	}

	var assigns []string
	hasError := false
	for _, destField := range destFields {
		sourceField := findSourceForDest(destField, byName, byTag, mapping.CustomFieldMappings, tag, sourceFields)
		additionalArg := findAdditionalArg(mapping.FuncAdditionalArgs, destField)
		assignment, returnsError := g.assignmentLine(sourceField, destField, g.conversions.Conversions, mapping.CustomConversions, additionalArg)
		if assignment != "" {
			assigns = append(assigns, assignment)
		}
		if returnsError {
			hasError = true
		}
	}

	fromTypeTemplate := mapping.From.TypeWithImportsTemplate
	toTypeTemplate := mapping.To.TypeWithImportsTemplate

	funcName := mapping.FuncName
	if funcName == "" {
		funcName = g.funcName(fromTypeTemplate, toTypeTemplate)
	}

	funcArgs := []string{fmt.Sprintf("src %s", fromTypeTemplate.ExecuteTemplate(g.importManager))}
	for _, arg := range mapping.FuncAdditionalArgs {
		funcArgs = append(funcArgs, arg.RenderParameter(g.importManager))
	}

	retType := toTypeTemplate.ExecuteTemplate(g.importManager)
	if hasError {
		return fmt.Sprintf(`// %s copies %s → %s
func %s(%s) (dst %s, err error) {
    %s
    return
}`, funcName, fromTypeTemplate.GetUnaliasedType(), toTypeTemplate.GetUnaliasedType(), funcName, strings.Join(funcArgs, ", "), retType, strings.Join(assigns, "\n\t")), nil
	} else {
		return fmt.Sprintf(`// %s copies %s → %s
func %s(%s) (dst %s) {
    %s
    return
}`, funcName, fromTypeTemplate.GetUnaliasedType(), toTypeTemplate.GetUnaliasedType(), funcName, strings.Join(funcArgs, ", "), retType, strings.Join(assigns, "\n\t")), nil
	}
}

func (g *Generator) funcName(fromType TypeWithImportsTemplate, toType TypeWithImportsTemplate) string {
	return fmt.Sprintf("Map%sTo%s", fromType.GetUnaliasedType(), toType.GetUnaliasedType())
}

func (g *Generator) assignmentLine(
	source *FieldDefinition,
	dest FieldDefinition,
	conversions []Conversion,
	customConversions []Conversion,
	additionalArg *AdditionalArg,
) (string, bool) {
	if additionalArg != nil {
		conversion, isReverse := g.findConversion(additionalArg.TypeWithImportsTemplate, dest.TypeWithImportsTemplate, conversions, customConversions)
		return g.assignmentWithConversion(
			additionalArg.Name,
			dest,
			conversion,
			isReverse,
		)
	} else if source != nil {
		conversion, isReverse := g.findConversion(source.TypeWithImportsTemplate, dest.TypeWithImportsTemplate, conversions, customConversions)

		return g.assignmentWithConversion(
			"src."+source.Name,
			dest,
			conversion,
			isReverse,
		)
	} else {
		return "// no matching source found for field: " + dest.Name + ", consider adding an additional arg or aligning the fields", false
	}
}

func (g *Generator) assignmentWithConversion(sourceExpr string, dest FieldDefinition, conversion *Conversion, isReverse bool) (string, bool) {
	destExpr := fmt.Sprintf("dst.%s", dest.Name)
	errorExpr := "err"
	if conversion != nil {
		if isReverse {
			return conversion.ExecuteReverseConversionTemplate(sourceExpr, destExpr, errorExpr, g.importManager)
		} else {
			return conversion.ExecuteConversionTemplate(sourceExpr, destExpr, errorExpr, g.importManager)
		}
	}
	return fmt.Sprintf("%s = %s", destExpr, sourceExpr), false
}

func (g *Generator) findConversion(
	sourceTypeTemplate TypeWithImportsTemplate,
	destTypeTemplate TypeWithImportsTemplate,
	conversions []Conversion,
	customConversions []Conversion,
) (*Conversion, bool) {
	equalsFunc := func(conv Conversion, sourceTypeTemplate TypeWithImportsTemplate, destTypeTemplate TypeWithImportsTemplate) bool {
		return conv.GetSourceTypeWithImportsTemplate().Equals(sourceTypeTemplate, g.importManager) && conv.GetDestTypeWithImportsTemplate().Equals(destTypeTemplate, g.importManager)
	}
	reverseEqualsFunc := func(conv Conversion, sourceTypeTemplate TypeWithImportsTemplate, destTypeTemplate TypeWithImportsTemplate) bool {
		return conv.GetDestTypeWithImportsTemplate().Equals(sourceTypeTemplate, g.importManager) && conv.GetSourceTypeWithImportsTemplate().Equals(destTypeTemplate, g.importManager) && conv.ReverseConversion.Tmpl != ""
	}
	for _, conv := range customConversions {
		if equalsFunc(conv, sourceTypeTemplate, destTypeTemplate) {
			return &conv, false
		}
		if reverseEqualsFunc(conv, sourceTypeTemplate, destTypeTemplate) {
			return &conv, true
		}
	}
	for _, conv := range conversions {
		if equalsFunc(conv, sourceTypeTemplate, destTypeTemplate) {
			return &conv, false
		}
		if reverseEqualsFunc(conv, sourceTypeTemplate, destTypeTemplate) {
			return &conv, true
		}
	}
	return nil, false
}

func (g *Generator) findStructDefinition(pkgPath string, typeName string) (*ast.StructType, string, error) {
	visited := map[string]bool{}
	return g.findStructDefinitionRecursive(pkgPath, typeName, visited)
}

func (g *Generator) findStructDefinitionRecursive(
	pkgPath string,
	typeName string,
	visited map[string]bool,
) (*ast.StructType, string, error) {
	key := fmt.Sprintf("%s.%s", pkgPath, typeName)
	if visited[key] {
		return nil, "", fmt.Errorf("circular type alias detected: %s", key)
	}
	visited[key] = true

	pkg, err := g.packageManager.GetPackage(pkgPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load package %s: %w", pkgPath, err)
	}

	fset := token.NewFileSet()
	for _, file := range pkg.GoFiles {
		f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			continue
		}

		var foundStruct *ast.StructType
		var foundPkgPath string
		var foundErr error

		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if ts.Name.Name != typeName {
				return true
			}

			switch t := ts.Type.(type) {
			case *ast.StructType:
				foundStruct = t
				foundPkgPath = pkgPath
				return false
			case *ast.Ident:
				aliasTypeName := t.Name

				if strings.Contains(aliasTypeName, ".") {
					parts := strings.Split(aliasTypeName, ".")
					if len(parts) != 2 {
						foundErr = fmt.Errorf("invalid qualified type: %s", aliasTypeName)
						return false
					}
					importPkgPath := parts[0]
					importTypeName := parts[1]

					importInfo, err := g.findImportSpecForAlias(f, importPkgPath)
					if err != nil {
						foundErr = fmt.Errorf("import path not found for %s", importPkgPath)
						return false
					}

					recursiveStruct, recursivePkgPath, recursiveErr := g.findStructDefinitionRecursive(importInfo.Path, importTypeName, visited)
					if recursiveErr != nil {
						foundErr = recursiveErr
						return false
					}
					foundStruct = recursiveStruct
					foundPkgPath = recursivePkgPath
					return false
				} else {
					recursiveStruct, recursivePkgPath, recursiveErr := g.findStructDefinitionRecursive(pkgPath, aliasTypeName, visited)
					if recursiveErr != nil {
						foundErr = recursiveErr
						return false
					}
					foundStruct = recursiveStruct
					foundPkgPath = recursivePkgPath
					return false
				}
			}
			return true
		})

		if foundStruct != nil {
			return foundStruct, foundPkgPath, nil
		}
		if foundErr != nil {
			return nil, "", foundErr
		}
	}

	return nil, "", fmt.Errorf("type %s not found in package %s", typeName, pkgPath)
}

func (g *Generator) findImportSpecForAlias(f *ast.File, pkgAlias string) (*ImportInfo, error) {
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, "\"")
		pkg, err := g.packageManager.GetPackage(path)
		if err != nil {
			return nil, err
		}
		if imp.Name != nil && imp.Name.Name == pkgAlias {
			return &ImportInfo{
				Alias:   &imp.Name.Name,
				PkgName: pkg.Name,
				Path:    pkg.PkgPath,
			}, nil
		}
		if pkg.Name == pkgAlias {
			return &ImportInfo{
				Alias:   nil,
				PkgName: pkg.Name,
				Path:    pkg.PkgPath,
			}, nil
		}
	}
	return nil, nil
}

func (g *Generator) findImportSpecsForExpression(expression ast.Expr, pkgPath string) ([]ImportInfo, error) {
	result := []ImportInfo{}

	pkgAliases, err := pkgAliasVisitor(expression)
	if err != nil {
		return nil, fmt.Errorf("failed to parse type expression: %w", err)
	}

	if len(pkgAliases) == 0 {
		return result, nil
	}

	pkg, err := g.packageManager.GetPackage(pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load package %s: %w", pkgPath, err)
	}

	fset := token.NewFileSet()
	for _, pkgAlias := range pkgAliases {
		found := false
		for _, gofile := range pkg.GoFiles {
			file, err := parser.ParseFile(fset, gofile, nil, parser.ParseComments)
			if err != nil {
				continue
			}
			importInfo, err := g.findImportSpecForAlias(file, pkgAlias)
			if err != nil {
				return nil, err
			}
			if importInfo == nil {
				continue
			}
			result = append(result, *importInfo)

			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf("import not found for package %s in %s", pkgAlias, pkgPath)
		}
	}
	return result, nil
}

func (g *Generator) resolveTypeForEmbeddedField(expression ast.Expr, currentPkgPath string) (string, string, error) {
	switch e := expression.(type) {
	case *ast.StarExpr:
		return g.resolveTypeForEmbeddedField(e.X, currentPkgPath)
	case *ast.Ident:
		return currentPkgPath, e.Name, nil
	case *ast.SelectorExpr:
		ident, ok := e.X.(*ast.Ident)
		if !ok {
			return "", "", fmt.Errorf("unsupported selector expression for embedded field")
		}
		pkg, err := g.packageManager.GetPackage(currentPkgPath)
		if err != nil {
			return "", "", fmt.Errorf("failed to load package %s: %w", currentPkgPath, err)
		}
		fset := token.NewFileSet()
		for _, gofile := range pkg.GoFiles {
			file, err := parser.ParseFile(fset, gofile, nil, parser.ParseComments)
			if err != nil {
				continue
			}
			importInfo, err := g.findImportSpecForAlias(file, ident.Name)
			if err != nil {
				return "", "", err
			}
			if importInfo != nil {
				return importInfo.Path, e.Sel.Name, nil
			}
		}
		return "", "", fmt.Errorf("import not found for package %s in %s", ident.Name, currentPkgPath)
	default:
		return "", "", fmt.Errorf("unsupported embedded field type")
	}
}

func (g *Generator) expandEmbeddedFields(fld *ast.Field, structPkgPath string) ([]FieldDefinition, error) {
	pkgPath, typeName, err := g.resolveTypeForEmbeddedField(fld.Type, structPkgPath)
	if err != nil {
		return nil, err
	}
	return g.extractFieldsFromPackage(pkgPath, typeName)
}

func pkgAliasVisitor(expression ast.Expr) ([]string, error) {
	pkgAliases := []string{}
	seen := map[string]struct{}{}

	var visit func(ast.Expr)
	visit = func(e ast.Expr) {
		switch v := e.(type) {
		case *ast.SelectorExpr:
			if ident, ok := v.X.(*ast.Ident); ok {
				if _, already := seen[ident.Name]; !already {
					pkgAliases = append(pkgAliases, ident.Name)
					seen[ident.Name] = struct{}{}
				}
			}
			visit(v.Sel)
		case *ast.StarExpr:
			visit(v.X)
		case *ast.ArrayType:
			visit(v.Elt)
		case *ast.MapType:
			visit(v.Key)
			visit(v.Value)
		case *ast.StructType:
			for _, f := range v.Fields.List {
				visit(f.Type)
			}
		case *ast.FuncType:
			if v.Params != nil {
				for _, f := range v.Params.List {
					visit(f.Type)
				}
			}
			if v.Results != nil {
				for _, f := range v.Results.List {
					visit(f.Type)
				}
			}
		}
	}
	visit(expression)
	return pkgAliases, nil
}

func findAdditionalArg(additionalArgs []AdditionalArg, dest FieldDefinition) *AdditionalArg {
	for _, arg := range additionalArgs {
		if arg.DestField == dest.Name {
			return &arg
		}
	}
	return nil
}

func tagValue(tag string, key string) string {
	if tag == "" {
		return ""
	}
	st := reflect.StructTag(tag)
	v := st.Get(key)
	if v == "" {
		return ""
	}
	parts := strings.Split(v, ",")
	if parts[0] == "-" {
		return ""
	}
	return parts[0]
}

func findSourceForDest(
	dest FieldDefinition,
	byName, byTag map[string]FieldDefinition,
	customFieldMappings []CustomFieldMapping,
	tag string,
	sourceFields []FieldDefinition,
) *FieldDefinition {
	for _, customFieldMapping := range customFieldMappings {
		if customFieldMapping.DestField != "" && customFieldMapping.DestField == dest.Name && customFieldMapping.SourceField != "" {
			if field, ok := byName[customFieldMapping.SourceField]; ok {
				return &field
			}
		}
		if customFieldMapping.DestTag != "" {
			customTag := customFieldMapping.Tag
			if customTag == "" {
				customTag = tag
			}
			if tagVal := tagValue(dest.Tag, customTag); tagVal != "" && tagVal == customFieldMapping.DestTag {
				if customFieldMapping.SourceTag != "" {
					for _, field := range sourceFields {
						if tagValue(field.Tag, customTag) == customFieldMapping.SourceTag {
							return &field
						}
					}
				}
			}
		}
	}

	if field, ok := byName[dest.Name]; ok {
		return &field
	}
	if tagVal := tagValue(dest.Tag, tag); tagVal != "" {
		if field, ok := byTag[tagVal]; ok {
			return &field
		}
	}
	return nil
}
