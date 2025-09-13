package packages

import (
	"fmt"

	"golang.org/x/tools/go/packages"
)

type PackageManager struct {
	packageCache map[string]*packages.Package
}

func NewPackageManager() *PackageManager {
	return &PackageManager{
		packageCache: make(map[string]*packages.Package),
	}
}

func (pm *PackageManager) GetPackage(pkgPath string) (*packages.Package, error) {
	if pkg, exists := pm.packageCache[pkgPath]; exists {
		return pkg, nil
	}

	pkg, err := loadPackage(pkgPath)

	pm.packageCache[pkgPath] = pkg
	return pkg, err
}

func loadPackage(pkgPath string) (*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedFiles | packages.NeedName,
	}
	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load package %s: %w", pkgPath, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("package not found: %s", pkgPath)
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("package errors: %v", pkg.Errors)
	}

	return pkg, nil
}
