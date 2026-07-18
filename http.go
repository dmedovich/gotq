package query

import (
	"fmt"
	"net/url"
	"strconv"
	"unicode/utf8"
)

type parseConfig struct {
	limits               Limits
	compatibilityAliases bool
}

// ParseOption configures ParseHTTP. Implementations are sealed.
type ParseOption interface {
	applyParse(*parseConfig)
}

type parseOptionFunc func(*parseConfig)

func (f parseOptionFunc) applyParse(config *parseConfig) { f(config) }

// WithLimits replaces every default query limit. All values must be positive.
func WithLimits(limits Limits) ParseOption {
	return parseOptionFunc(func(config *parseConfig) { config.limits = limits })
}

// WithCompatibilityAliases accepts the pre-release orderby/top/skip parameter
// names in addition to the canonical sort/limit/offset names.
func WithCompatibilityAliases() ParseOption {
	return parseOptionFunc(func(config *parseConfig) { config.compatibilityAliases = true })
}

// ParseHTTP parses the V1 query parameters from decoded URL values. It does
// not know a model schema and does not access a database.
func ParseHTTP(values url.Values, options ...ParseOption) (Query, error) {
	config := parseConfig{limits: defaultQueryLimits}
	for _, option := range options {
		if option == nil {
			return Query{}, schemaError("parse option is nil", "")
		}
		option.applyParse(&config)
	}
	if config.limits.MaxQueryBytes <= 0 || config.limits.MaxFilterBytes <= 0 || config.limits.MaxTokens <= 0 || config.limits.MaxLiteralBytes <= 0 || config.limits.MaxInValues <= 0 || config.limits.MaxLimit <= 0 || config.limits.MaxOffset <= 0 || config.limits.MaxSortTerms <= 0 || config.limits.MaxSearchBytes <= 0 || config.limits.MaxExpressionDepth <= 0 || config.limits.MaxNodes <= 0 || config.limits.MaxPathDepth <= 0 || config.limits.MaxQuantifierDepth <= 0 || config.limits.MaxCursorBytes <= 0 {
		return Query{}, schemaError("query limits must all be positive", "")
	}
	if decodedQueryBytes(values) > config.limits.MaxQueryBytes {
		return Query{}, positionedError(CodeLimitExceeded, "query", "", 0, fmt.Sprintf("decoded query must not exceed %d bytes", config.limits.MaxQueryBytes))
	}

	query := Query{limits: config.limits}
	filter, present, err := singleParameter(values, "filter")
	if err != nil {
		return Query{}, err
	}
	if present {
		if len(filter) > config.limits.MaxFilterBytes {
			return Query{}, positionedError(CodeLimitExceeded, "filter", filter, config.limits.MaxFilterBytes, fmt.Sprintf("filter must not exceed %d bytes", config.limits.MaxFilterBytes))
		}
		expr, parseErr := parseFilter(filter, config.limits)
		if parseErr != nil {
			return Query{}, parseErr
		}
		query.Filter = expr
		query.filterSource = filter
	}

	sort, _, legacy, present, err := aliasedParameter(values, "sort", "orderby", config.compatibilityAliases)
	if err != nil {
		return Query{}, err
	}
	if present {
		var terms []SortTerm
		var sortErr *Error
		if legacy {
			terms, sortErr = parseLegacyOrderBy(sort, config.limits.MaxPathDepth)
		} else {
			terms, sortErr = parseSort(sort, config.limits.MaxSortTerms, config.limits.MaxPathDepth)
		}
		if sortErr != nil {
			return Query{}, sortErr
		}
		if len(terms) > config.limits.MaxSortTerms {
			return Query{}, positionedError(CodeLimitExceeded, "orderby", sort, terms[config.limits.MaxSortTerms].Source.Start, fmt.Sprintf("orderby must not contain more than %d terms", config.limits.MaxSortTerms))
		}
		query.Sort = terms
		query.sortSource = sort
	}

	limit, parameter, _, present, err := aliasedParameter(values, "limit", "top", config.compatibilityAliases)
	if err != nil {
		return Query{}, err
	}
	if present {
		value, numberErr := parseUnsignedParameter(parameter, limit)
		if numberErr != nil {
			return Query{}, numberErr
		}
		if value > config.limits.MaxLimit {
			return Query{}, positionedError(CodeLimitExceeded, parameter, limit, 0, fmt.Sprintf("%s must not exceed %d", parameter, config.limits.MaxLimit))
		}
		query.Limit = &value
	}

	offset, parameter, _, present, err := aliasedParameter(values, "offset", "skip", config.compatibilityAliases)
	if err != nil {
		return Query{}, err
	}
	if present {
		value, numberErr := parseUnsignedParameter(parameter, offset)
		if numberErr != nil {
			return Query{}, numberErr
		}
		if value > config.limits.MaxOffset {
			return Query{}, positionedError(CodeLimitExceeded, parameter, offset, 0, fmt.Sprintf("%s must not exceed %d", parameter, config.limits.MaxOffset))
		}
		query.Offset = &value
	}

	cursor, present, err := singleParameter(values, "cursor")
	if err != nil {
		return Query{}, err
	}
	if present {
		if len(cursor) > config.limits.MaxCursorBytes {
			return Query{}, positionedError(CodeLimitExceeded, "cursor", cursor, config.limits.MaxCursorBytes, fmt.Sprintf("cursor must not exceed %d bytes", config.limits.MaxCursorBytes))
		}
		for index := range len(cursor) {
			if !isBase64URLCharacter(cursor[index]) {
				return Query{}, positionedError(CodeInvalidCursor, "cursor", cursor, index, "cursor must use unpadded base64url characters")
			}
		}
		if query.Offset != nil {
			return Query{}, positionedError(CodeInvalidParameter, "cursor", cursor, 0, "cursor and offset cannot be used together")
		}
		query.Cursor = &cursor
	}

	count, present, err := singleParameter(values, "count")
	if err != nil {
		return Query{}, err
	}
	if present {
		var value bool
		switch count {
		case "true":
			value = true
		case "false":
			value = false
		default:
			return Query{}, positionedError(CodeInvalidParameter, "count", count, 0, "count must be exactly true or false")
		}
		query.Count = &value
	}

	search, present, err := singleParameter(values, "search")
	if err != nil {
		return Query{}, err
	}
	if present {
		if !utf8.ValidString(search) {
			return Query{}, positionedError(CodeInvalidParameter, "search", search, 0, "search must be valid UTF-8")
		}
		if len(search) > config.limits.MaxSearchBytes {
			return Query{}, positionedError(CodeLimitExceeded, "search", search, 0, fmt.Sprintf("search must not exceed %d bytes", config.limits.MaxSearchBytes))
		}
		query.Search = &search
	}

	return query, nil
}

func isBase64URLCharacter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '-' || value == '_'
}

func decodedQueryBytes(values url.Values) int {
	total := 0
	for name, entries := range values {
		total += len(name)
		for _, entry := range entries {
			total += len(entry)
		}
	}
	return total
}

func aliasedParameter(values url.Values, canonical, alias string, allowAlias bool) (value, parameter string, legacy, present bool, err *Error) {
	canonicalValues, hasCanonical := values[canonical]
	aliasValues, hasAlias := values[alias]
	if !allowAlias {
		hasAlias = false
	}
	if hasCanonical && hasAlias {
		return "", canonical, false, false, positionedError(CodeInvalidParameter, canonical, "", 0, fmt.Sprintf("parameters %q and %q cannot be used together", canonical, alias))
	}
	if hasCanonical {
		value, present, err = singleParameter(url.Values{canonical: canonicalValues}, canonical)
		return value, canonical, false, present, err
	}
	if hasAlias {
		value, present, err = singleParameter(url.Values{alias: aliasValues}, alias)
		return value, alias, true, present, err
	}
	return "", canonical, false, false, nil
}

func singleParameter(values url.Values, name string) (string, bool, *Error) {
	entries, present := values[name]
	if !present {
		return "", false, nil
	}
	if len(entries) != 1 {
		return "", false, positionedError(CodeInvalidParameter, name, "", 0, fmt.Sprintf("parameter %q must occur exactly once", name))
	}
	if entries[0] == "" {
		return "", false, positionedError(CodeInvalidParameter, name, "", 0, fmt.Sprintf("parameter %q must not be empty", name))
	}
	return entries[0], true, nil
}

func parseUnsignedParameter(name, input string) (int, *Error) {
	if input == "0" {
		return 0, nil
	}
	if input == "" || input[0] < '1' || input[0] > '9' {
		return 0, positionedError(CodeInvalidParameter, name, input, 0, fmt.Sprintf("%s must be a non-negative base-10 integer", name))
	}
	for i := 1; i < len(input); i++ {
		if !isDigit(input[i]) {
			return 0, positionedError(CodeInvalidParameter, name, input, i, fmt.Sprintf("%s must contain only decimal digits", name))
		}
	}
	value, err := strconv.ParseUint(input, 10, 64)
	if err != nil || value > uint64(maxInt()) {
		return 0, positionedError(CodeInvalidParameter, name, input, 0, fmt.Sprintf("%s does not fit in an int", name))
	}
	return int(value), nil
}

func maxInt() int { return int(^uint(0) >> 1) }

func parseSort(input string, maxTerms, maxPathDepth int) ([]SortTerm, *Error) {
	position := 0
	skipSpacing := func() {
		for position < len(input) && isWhitespace(input[position]) {
			position++
		}
	}
	skipSpacing()
	if position == len(input) {
		return nil, positionedError(CodeInvalidParameter, "sort", input, position, "sort requires at least one term")
	}
	seen := make(map[string]struct{})
	var terms []SortTerm
	for {
		termStart := position
		desc := false
		if input[position] == '-' {
			desc = true
			position++
		}
		nameStart := position
		if err := scanSortPath("sort", input, &position, maxPathDepth); err != nil {
			return nil, err
		}
		name := input[nameStart:position]
		if _, duplicate := seen[name]; duplicate {
			return nil, positionedError(CodeInvalidParameter, "sort", input, nameStart, fmt.Sprintf("sort field %q is repeated", name))
		}
		seen[name] = struct{}{}
		skipSpacing()
		terms = append(terms, SortTerm{Field: name, Desc: desc, Source: Span{Start: termStart, End: position}})
		if len(terms) > maxTerms {
			return nil, positionedError(CodeLimitExceeded, "sort", input, termStart, fmt.Sprintf("sort must not contain more than %d terms", maxTerms))
		}
		if position == len(input) {
			return terms, nil
		}
		if input[position] != ',' {
			return nil, sortCharacterError("sort", input, position, "expected comma between sort terms")
		}
		position++
		skipSpacing()
		if position == len(input) {
			return nil, positionedError(CodeInvalidParameter, "sort", input, position, "expected sort term after comma")
		}
	}
}

func parseLegacyOrderBy(input string, maxPathDepth int) ([]SortTerm, *Error) {
	position := 0
	skipSpacing := func() {
		for position < len(input) && isWhitespace(input[position]) {
			position++
		}
	}
	skipSpacing()
	if position == len(input) {
		return nil, positionedError(CodeInvalidParameter, "orderby", input, position, "orderby requires at least one term")
	}

	seen := make(map[string]struct{})
	var terms []SortTerm
	for {
		termStart := position
		nameStart := position
		if err := scanSortPath("orderby", input, &position, maxPathDepth); err != nil {
			return nil, err
		}
		name := input[nameStart:position]
		if _, duplicate := seen[name]; duplicate {
			return nil, positionedError(CodeInvalidParameter, "orderby", input, nameStart, fmt.Sprintf("orderby field %q is repeated", name))
		}
		seen[name] = struct{}{}

		beforeSpacing := position
		skipSpacing()
		desc := false
		if position < len(input) && input[position] != ',' {
			if beforeSpacing == position {
				return nil, orderByCharacterError(input, position, "expected comma or whitespace before sort direction")
			}
			directionStart := position
			if !isIdentifierStart(input[position]) {
				return nil, orderByCharacterError(input, position, "expected asc or desc")
			}
			position++
			for position < len(input) && isIdentifierContinue(input[position]) {
				position++
			}
			direction := input[directionStart:position]
			switch direction {
			case "asc":
			case "desc":
				desc = true
			default:
				return nil, positionedError(CodeInvalidParameter, "orderby", input, directionStart, fmt.Sprintf("unknown sort direction %q", direction))
			}
			skipSpacing()
		}
		terms = append(terms, SortTerm{Field: name, Desc: desc, Source: Span{Start: termStart, End: position}})

		if position == len(input) {
			return terms, nil
		}
		if input[position] != ',' {
			return nil, orderByCharacterError(input, position, "expected comma between order terms")
		}
		position++
		skipSpacing()
		if position == len(input) {
			return nil, positionedError(CodeInvalidParameter, "orderby", input, position, "expected order term after comma")
		}
	}
}

func scanSortPath(parameter, input string, position *int, maxPathDepth int) *Error {
	depth := 0
	for {
		if *position >= len(input) || !isIdentifierStart(input[*position]) {
			return sortCharacterError(parameter, input, *position, "expected sort field")
		}
		depth++
		if depth > maxPathDepth {
			return positionedError(CodeLimitExceeded, parameter, input, *position, fmt.Sprintf("sort path depth exceeds %d", maxPathDepth))
		}
		*position++
		for *position < len(input) && isIdentifierContinue(input[*position]) {
			*position++
		}
		if *position >= len(input) || input[*position] != '/' {
			return nil
		}
		*position++
	}
}

func orderByCharacterError(input string, offset int, message string) *Error {
	return sortCharacterError("orderby", input, offset, message)
}

func sortCharacterError(parameter, input string, offset int, message string) *Error {
	if offset < len(input) && input[offset] >= utf8.RuneSelf {
		_, size := utf8.DecodeRuneInString(input[offset:])
		if size == 1 {
			message = "invalid UTF-8 encoding"
		} else {
			message = "order fields must contain only ASCII letters, digits, and underscore"
		}
	}
	return positionedError(CodeInvalidParameter, parameter, input, offset, message)
}
