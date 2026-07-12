// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package fibergen

// PreferAnnotation returns the annotated value when present.
// When inferred and annotated values conflict, annotation wins.
func PreferAnnotation(inferred, annotated string) string {
	if annotated != "" {
		return annotated
	}
	return inferred
}

// PreferAnnotationMap merges status-keyed schemas; annotated entries override inferred.
func PreferAnnotationMap(inferred, annotated map[string]string) map[string]string {
	if len(inferred) == 0 && len(annotated) == 0 {
		return nil
	}
	out := make(map[string]string, len(inferred)+len(annotated))
	for k, v := range inferred {
		out[k] = v
	}
	for k, v := range annotated {
		if v != "" {
			out[k] = v
		}
	}
	return out
}
