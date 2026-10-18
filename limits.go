package query

// intersectLimits retains an explicit parser policy without imposing the
// global defaults on a manually constructed Query (whose limits are zero).
func intersectLimits(endpoint, parser Limits) Limits {
	if parser == (Limits{}) {
		return endpoint
	}
	return Limits{
		MaxQueryBytes:      min(endpoint.MaxQueryBytes, parser.MaxQueryBytes),
		MaxFilterBytes:     min(endpoint.MaxFilterBytes, parser.MaxFilterBytes),
		MaxTokens:          min(endpoint.MaxTokens, parser.MaxTokens),
		MaxLiteralBytes:    min(endpoint.MaxLiteralBytes, parser.MaxLiteralBytes),
		MaxInValues:        min(endpoint.MaxInValues, parser.MaxInValues),
		MaxLimit:           min(endpoint.MaxLimit, parser.MaxLimit),
		MaxOffset:          min(endpoint.MaxOffset, parser.MaxOffset),
		MaxSortTerms:       min(endpoint.MaxSortTerms, parser.MaxSortTerms),
		MaxSearchBytes:     min(endpoint.MaxSearchBytes, parser.MaxSearchBytes),
		MaxExpressionDepth: min(endpoint.MaxExpressionDepth, parser.MaxExpressionDepth),
		MaxNodes:           min(endpoint.MaxNodes, parser.MaxNodes),
		MaxPathDepth:       min(endpoint.MaxPathDepth, parser.MaxPathDepth),
		MaxQuantifierDepth: min(endpoint.MaxQuantifierDepth, parser.MaxQuantifierDepth),
		MaxCursorBytes:     min(endpoint.MaxCursorBytes, parser.MaxCursorBytes),
	}
}
