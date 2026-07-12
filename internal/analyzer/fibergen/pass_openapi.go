// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

// OpenAPIAssemblerPass builds the document draft from prior passes (Pass 5).
func OpenAPIAssemblerPass(data *CollectedData) DocumentDraft {
	if data == nil {
		return DocumentDraft{}
	}
	return DocumentDraft{
		Title:           defaultTitle(data.PkgName),
		Version:         "1.0.0",
		Routes:          data.Routes,
		Schemas:         data.Schemas,
		SecuritySchemes: data.SecuritySchemes,
	}
}

func defaultTitle(pkgName string) string {
	if pkgName == "" {
		return "API"
	}
	return pkgName + " API"
}
