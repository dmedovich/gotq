package query

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type fieldConfig struct {
	filterable    bool
	filterableSet bool
	sortable      bool
	sortableSet   bool
	operators     []ComparisonOperator
	goField       string
	goFieldSet    bool
	column        string
	columnSet     bool
	codec         ScalarCodec
	codecSet      bool
	err           string
}

// ScalarCodec converts syntax into a typed bound value for a custom scalar. It
// has no access to SQL compilation or identifiers.
type ScalarCodec interface {
	Name() string
	ValidateType(reflect.Type) error
	ParseLiteral(Literal, reflect.Type) (any, error)
}

// FieldOption configures one schema field. Implementations are sealed.
type FieldOption interface {
	applyField(*fieldConfig)
}

type fieldOptionFunc func(*fieldConfig)

func (f fieldOptionFunc) applyField(config *fieldConfig) { f(config) }

// Filterable enables filtering. With no arguments it selects the kind's
// default operators; arguments replace that default with an explicit subset.
func Filterable(operators ...ComparisonOperator) FieldOption {
	copyOfOperators := append([]ComparisonOperator(nil), operators...)
	return fieldOptionFunc(func(config *fieldConfig) {
		if config.filterableSet {
			config.err = "Filterable is specified more than once"
			return
		}
		config.filterableSet = true
		config.filterable = true
		config.operators = copyOfOperators
	})
}

// Sortable enables ordering by the field.
func Sortable() FieldOption {
	return fieldOptionFunc(func(config *fieldConfig) {
		if config.sortableSet {
			config.err = "Sortable is specified more than once"
			return
		}
		config.sortableSet = true
		config.sortable = true
	})
}

// GoField explicitly selects an exported top-level Go struct field.
func GoField(name string) FieldOption {
	return fieldOptionFunc(func(config *fieldConfig) {
		if config.goFieldSet {
			config.err = "GoField is specified more than once"
			return
		}
		config.goFieldSet = true
		config.goField = name
	})
}

// Column explicitly selects the whitelisted database column.
func Column(name string) FieldOption {
	return fieldOptionFunc(func(config *fieldConfig) {
		if config.columnSet {
			config.err = "Column is specified more than once"
			return
		}
		config.columnSet = true
		config.column = name
	})
}

// WithCodec assigns a constrained custom scalar codec to a field.
func WithCodec(codec ScalarCodec) FieldOption {
	return fieldOptionFunc(func(config *fieldConfig) {
		if config.codecSet {
			config.err = "WithCodec is specified more than once"
			return
		}
		config.codecSet = true
		config.codec = codec
	})
}

type fieldDeclaration struct {
	publicName string
	kind       Kind
	inferKind  bool
	options    []FieldOption
}

type relationshipConfig struct {
	goField    string
	goFieldSet bool
	err        string
}

// RelationshipOption configures one explicitly exposed relationship.
// Implementations are sealed.
type RelationshipOption interface {
	applyRelationship(*relationshipConfig)
}

type relationshipOptionFunc func(*relationshipConfig)

func (f relationshipOptionFunc) applyRelationship(config *relationshipConfig) { f(config) }

// RelationGoField explicitly selects an exported top-level Go association
// field when the public relationship name does not match its JSON name.
func RelationGoField(name string) RelationshipOption {
	return relationshipOptionFunc(func(config *relationshipConfig) {
		if config.goFieldSet {
			config.err = "RelationGoField is specified more than once"
			return
		}
		config.goFieldSet = true
		config.goField = name
	})
}

// NestedPolicy is a sealed relationship target policy. Values returned by
// Schema[T] implement it; arbitrary implementations are not accepted.
type NestedPolicy interface {
	nestedPolicy()
}

type relationshipDeclaration struct {
	publicName string
	policy     NestedPolicy
	options    []RelationshipOption
}

// SchemaBuilder accumulates model field declarations. Call Build once startup
// configuration is complete; the resulting ModelSchema is immutable.
type SchemaBuilder[T any] struct {
	fields        []fieldDeclaration
	relationships []relationshipDeclaration
}

// ModelSchema is an immutable whitelist and type map for one model.
type ModelSchema[T any] struct {
	*modelSchema
}

type modelSchema struct {
	modelType      reflect.Type
	fields         map[string]*modelField
	primaryColumns []string
	table          string
	relationships  map[string]*modelRelationship
	gormSchema     *schema.Schema
}

type modelRelationship struct {
	publicName  string
	goName      string
	cardinality RelationshipCardinality
	target      *modelSchema
	metadata    *schema.Relationship
}

type modelField struct {
	publicName  string
	goName      string
	column      string
	kind        Kind
	scalarType  reflect.Type
	bits        int
	nullable    bool
	filterable  bool
	sortable    bool
	operators   []ComparisonOperator
	operatorSet map[ComparisonOperator]struct{}
	codec       ScalarCodec
	codecName   string
}

// Schema starts a schema builder for T.
func Schema[T any]() *SchemaBuilder[T] {
	return &SchemaBuilder[T]{}
}

// Field adds a public model field declaration. Errors are accumulated as
// declarations and reported deterministically by Build.
func (b *SchemaBuilder[T]) Field(publicName string, kind Kind, options ...FieldOption) *SchemaBuilder[T] {
	if b == nil {
		return b
	}
	b.fields = append(b.fields, fieldDeclaration{
		publicName: publicName,
		kind:       kind,
		options:    append([]FieldOption(nil), options...),
	})
	return b
}

// Expose adds a public field whose scalar kind and database column are inferred
// from the GORM model schema when Bind is called.
func (b *SchemaBuilder[T]) Expose(publicName string, options ...FieldOption) *SchemaBuilder[T] {
	if b == nil {
		return b
	}
	b.fields = append(b.fields, fieldDeclaration{
		publicName: publicName,
		inferKind:  true,
		options:    append([]FieldOption(nil), options...),
	})
	return b
}

// Relation explicitly exposes one GORM association and its nested public
// policy. Merely existing in GORM metadata never exposes a relationship.
func (b *SchemaBuilder[T]) Relation(publicName string, policy NestedPolicy, options ...RelationshipOption) *SchemaBuilder[T] {
	if b == nil {
		return b
	}
	b.relationships = append(b.relationships, relationshipDeclaration{
		publicName: publicName,
		policy:     policy,
		options:    append([]RelationshipOption(nil), options...),
	})
	return b
}

func (*SchemaBuilder[T]) nestedPolicy() {}

func (b *SchemaBuilder[T]) bindNested(db *gorm.DB, active map[uintptr]struct{}) (*modelSchema, error) {
	return b.buildCore(db, active)
}

func (b *SchemaBuilder[T]) policyIdentity() uintptr {
	if b == nil {
		return 0
	}
	return reflect.ValueOf(b).Pointer()
}

type nestedPolicyBinder interface {
	NestedPolicy
	bindNested(*gorm.DB, map[uintptr]struct{}) (*modelSchema, error)
	policyIdentity() uintptr
}

// Build validates reflection mappings and returns an immutable schema.
func (b *SchemaBuilder[T]) Build() (*ModelSchema[T], error) {
	return b.build(nil)
}

// Bind validates declarations against the configured GORM model schema. It
// supports inferred fields and honors the active naming strategy.
func (b *SchemaBuilder[T]) Bind(db *gorm.DB) (*ModelSchema[T], error) {
	if db == nil {
		return nil, schemaError("gorm DB is nil", "")
	}
	return b.build(db)
}

func (b *SchemaBuilder[T]) build(db *gorm.DB) (*ModelSchema[T], error) {
	core, err := b.buildCore(db, make(map[uintptr]struct{}))
	if err != nil {
		return nil, err
	}
	return &ModelSchema[T]{modelSchema: core}, nil
}

func (b *SchemaBuilder[T]) buildCore(db *gorm.DB, active map[uintptr]struct{}) (*modelSchema, error) {
	if b == nil {
		return nil, schemaError("schema builder is nil", "")
	}
	identity := b.policyIdentity()
	if _, cyclic := active[identity]; cyclic {
		return nil, schemaError("relationship policy contains a cycle", "")
	}
	active[identity] = struct{}{}
	defer delete(active, identity)
	modelType := reflect.TypeOf((*T)(nil)).Elem()
	if modelType.Kind() != reflect.Struct {
		return nil, schemaError(fmt.Sprintf("model type %s must be a struct", modelType), "")
	}

	var gormSchema *schema.Schema
	if db != nil {
		statement := &gorm.Statement{DB: db}
		if err := statement.Parse(reflect.New(modelType).Interface()); err != nil {
			return nil, schemaError(fmt.Sprintf("failed to parse GORM model schema: %v", err), "")
		}
		gormSchema = statement.Schema
		if gormSchema == nil {
			return nil, schemaError("GORM returned no model schema", "")
		}
	}

	fields := make(map[string]*modelField, len(b.fields))
	for _, declaration := range b.fields {
		if !isValidIdentifier(declaration.publicName) {
			return nil, schemaError(fmt.Sprintf("public field name %q is not a valid identifier", declaration.publicName), declaration.publicName, declaration.kind)
		}
		if isReservedName(declaration.publicName) {
			return nil, schemaError(fmt.Sprintf("public field name %q is reserved", declaration.publicName), declaration.publicName, declaration.kind)
		}
		if _, exists := fields[declaration.publicName]; exists {
			return nil, schemaError(fmt.Sprintf("public field %q is declared more than once", declaration.publicName), declaration.publicName, declaration.kind)
		}

		config := fieldConfig{}
		for _, option := range declaration.options {
			if option == nil {
				return nil, schemaError(fmt.Sprintf("field %q has a nil option", declaration.publicName), declaration.publicName, declaration.kind)
			}
			option.applyField(&config)
		}
		if config.err != "" {
			return nil, schemaError(fmt.Sprintf("field %q: %s", declaration.publicName, config.err), declaration.publicName, declaration.kind)
		}
		if config.goFieldSet && config.goField == "" {
			return nil, schemaError(fmt.Sprintf("field %q has an empty GoField option", declaration.publicName), declaration.publicName, declaration.kind)
		}
		if config.columnSet && config.column == "" {
			return nil, schemaError(fmt.Sprintf("field %q has an empty Column option", declaration.publicName), declaration.publicName, declaration.kind)
		}
		if config.codecSet && codecIsNil(config.codec) {
			return nil, schemaError(fmt.Sprintf("field %q has a nil custom codec", declaration.publicName), declaration.publicName)
		}

		goField, err := resolveGoField(modelType, declaration.publicName, config.goField)
		if err != nil {
			return nil, schemaError(err.Error(), declaration.publicName, declaration.kind)
		}
		kind := declaration.kind
		if config.codec != nil {
			kind = Custom
		} else if declaration.inferKind {
			kind, err = inferGoKind(goField.Type)
			if err != nil {
				return nil, schemaError(fmt.Sprintf("field %q: %v", declaration.publicName, err), declaration.publicName)
			}
		}
		var scalarType reflect.Type
		var bits int
		codecName := ""
		if config.codec != nil {
			scalarType = dereferenceType(goField.Type)
			codecName = strings.TrimSpace(config.codec.Name())
			if codecName == "" {
				return nil, schemaError(fmt.Sprintf("field %q custom codec has an empty name", declaration.publicName), declaration.publicName, Custom)
			}
			if codecErr := config.codec.ValidateType(scalarType); codecErr != nil {
				return nil, schemaError(fmt.Sprintf("field %q codec %q rejects Go type %s: %v", declaration.publicName, codecName, scalarType, codecErr), declaration.publicName, Custom)
			}
		} else {
			scalarType, bits, err = validateGoKind(goField.Type, kind)
		}
		if err != nil {
			return nil, schemaError(fmt.Sprintf("field %q: %v", declaration.publicName, err), declaration.publicName, kind)
		}

		column := config.column
		if column == "" && gormSchema != nil {
			gormField := gormSchema.LookUpField(goField.Name)
			if gormField == nil || gormField.DBName == "" || !gormField.Readable {
				return nil, schemaError(fmt.Sprintf("field %q does not resolve to a readable GORM column", declaration.publicName), declaration.publicName, kind)
			}
			column = gormField.DBName
		}
		if column == "" {
			column = gormColumn(goField.Tag.Get("gorm"))
		}
		if column == "" {
			return nil, schemaError(fmt.Sprintf("field %q has no explicit database column", declaration.publicName), declaration.publicName, declaration.kind)
		}
		if !isValidIdentifier(column) {
			return nil, schemaError(fmt.Sprintf("database column %q for field %q is not a simple identifier", column, declaration.publicName), declaration.publicName, declaration.kind)
		}

		operators := config.operators
		if config.filterable && len(operators) == 0 {
			operators = defaultOperators(kind)
			if goField.Type.Kind() == reflect.Pointer {
				operators = append(operators, IsNull, IsNotNull)
			}
		}
		operatorSet := make(map[ComparisonOperator]struct{}, len(operators))
		for _, operator := range operators {
			if (operator == IsNull || operator == IsNotNull) && goField.Type.Kind() != reflect.Pointer {
				return nil, schemaError(fmt.Sprintf("operator %q requires nullable field %q", operator, declaration.publicName), declaration.publicName, kind)
			}
			if operator != IsNull && operator != IsNotNull && !operatorCompatible(kind, operator) {
				return nil, schemaError(fmt.Sprintf("operator %q is incompatible with field %q of type %s", operator, declaration.publicName, kind), declaration.publicName, kind)
			}
			if _, duplicate := operatorSet[operator]; duplicate {
				return nil, schemaError(fmt.Sprintf("operator %q is repeated for field %q", operator, declaration.publicName), declaration.publicName, declaration.kind)
			}
			operatorSet[operator] = struct{}{}
		}

		fields[declaration.publicName] = &modelField{
			publicName:  declaration.publicName,
			goName:      goField.Name,
			column:      column,
			kind:        kind,
			scalarType:  scalarType,
			bits:        bits,
			nullable:    goField.Type.Kind() == reflect.Pointer,
			filterable:  config.filterable,
			sortable:    config.sortable,
			operators:   append([]ComparisonOperator(nil), operators...),
			operatorSet: operatorSet,
			codec:       config.codec,
			codecName:   codecName,
		}
	}
	var primaryColumns []string
	if gormSchema != nil {
		for _, field := range gormSchema.PrimaryFields {
			if field.DBName != "" {
				primaryColumns = append(primaryColumns, field.DBName)
			}
		}
	}
	core := &modelSchema{
		modelType:      modelType,
		fields:         fields,
		primaryColumns: primaryColumns,
		relationships:  make(map[string]*modelRelationship, len(b.relationships)),
		gormSchema:     gormSchema,
	}
	if gormSchema != nil {
		core.table = gormSchema.Table
	}
	if len(b.relationships) > 0 && gormSchema == nil {
		return nil, schemaError("relationship policies require Bind with a GORM database", "")
	}
	for _, declaration := range b.relationships {
		if !isValidIdentifier(declaration.publicName) {
			return nil, schemaError(fmt.Sprintf("public relationship name %q is not a valid identifier", declaration.publicName), declaration.publicName)
		}
		if isReservedName(declaration.publicName) {
			return nil, schemaError(fmt.Sprintf("public relationship name %q is reserved", declaration.publicName), declaration.publicName)
		}
		if _, exists := fields[declaration.publicName]; exists {
			return nil, schemaError(fmt.Sprintf("public name %q is declared as both a field and relationship", declaration.publicName), declaration.publicName)
		}
		if _, exists := core.relationships[declaration.publicName]; exists {
			return nil, schemaError(fmt.Sprintf("public relationship %q is declared more than once", declaration.publicName), declaration.publicName)
		}
		config := relationshipConfig{}
		for _, option := range declaration.options {
			if option == nil {
				return nil, schemaError(fmt.Sprintf("relationship %q has a nil option", declaration.publicName), declaration.publicName)
			}
			option.applyRelationship(&config)
		}
		if config.err != "" {
			return nil, schemaError(fmt.Sprintf("relationship %q: %s", declaration.publicName, config.err), declaration.publicName)
		}
		if config.goFieldSet && config.goField == "" {
			return nil, schemaError(fmt.Sprintf("relationship %q has an empty RelationGoField option", declaration.publicName), declaration.publicName)
		}
		goField, relationErr := resolveGoField(modelType, declaration.publicName, config.goField)
		if relationErr != nil {
			return nil, schemaError(relationErr.Error(), declaration.publicName)
		}
		metadata := gormSchema.Relationships.Relations[goField.Name]
		if metadata == nil || metadata.FieldSchema == nil || metadata.Field == nil || !metadata.Field.Readable {
			return nil, schemaError(fmt.Sprintf("relationship %q does not resolve to a readable GORM association", declaration.publicName), declaration.publicName)
		}
		if metadata.Polymorphic != nil {
			return nil, schemaError(fmt.Sprintf("relationship %q uses unsupported polymorphic metadata", declaration.publicName), declaration.publicName)
		}
		cardinality, relationErr := relationshipCardinality(metadata)
		if relationErr != nil {
			return nil, schemaError(fmt.Sprintf("relationship %q: %v", declaration.publicName, relationErr), declaration.publicName)
		}
		binder, ok := declaration.policy.(nestedPolicyBinder)
		if !ok || binder == nil || binder.policyIdentity() == 0 {
			return nil, schemaError(fmt.Sprintf("relationship %q has an invalid nested policy", declaration.publicName), declaration.publicName)
		}
		target, targetErr := binder.bindNested(db, active)
		if targetErr != nil {
			return nil, schemaError(fmt.Sprintf("relationship %q target policy: %v", declaration.publicName, targetErr), declaration.publicName)
		}
		if target.modelType != metadata.FieldSchema.ModelType {
			return nil, schemaError(fmt.Sprintf("relationship %q target policy model %s does not match GORM association model %s", declaration.publicName, target.modelType, metadata.FieldSchema.ModelType), declaration.publicName)
		}
		if relationErr := validateRelationshipMetadata(metadata); relationErr != nil {
			return nil, schemaError(fmt.Sprintf("relationship %q: %v", declaration.publicName, relationErr), declaration.publicName)
		}
		core.relationships[declaration.publicName] = &modelRelationship{
			publicName:  declaration.publicName,
			goName:      goField.Name,
			cardinality: cardinality,
			target:      target,
			metadata:    metadata,
		}
	}
	return core, nil
}

func relationshipCardinality(relationship *schema.Relationship) (RelationshipCardinality, error) {
	switch relationship.Type {
	case schema.HasOne, schema.BelongsTo:
		return RelationshipOne, nil
	case schema.HasMany, schema.Many2Many:
		return RelationshipMany, nil
	default:
		return "", fmt.Errorf("unsupported GORM relationship type %q", relationship.Type)
	}
}

func validateRelationshipMetadata(relationship *schema.Relationship) error {
	if relationship.Schema == nil || relationship.FieldSchema == nil || relationship.Schema.Table == "" || relationship.FieldSchema.Table == "" {
		return fmt.Errorf("GORM relationship has incomplete table metadata")
	}
	if len(relationship.References) == 0 {
		return fmt.Errorf("GORM relationship has no key references")
	}
	if relationship.Type == schema.Many2Many && (relationship.JoinTable == nil || relationship.JoinTable.Table == "") {
		return fmt.Errorf("many-to-many relationship has no join-table metadata")
	}
	for _, reference := range relationship.References {
		if reference == nil || reference.ForeignKey == nil || reference.ForeignKey.DBName == "" {
			return fmt.Errorf("GORM relationship has an incomplete foreign-key reference")
		}
		if reference.PrimaryValue == "" && (reference.PrimaryKey == nil || reference.PrimaryKey.DBName == "") {
			return fmt.Errorf("GORM relationship has an incomplete primary-key reference")
		}
	}
	return nil
}

func inferGoKind(fieldType reflect.Type) (Kind, error) {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	switch fieldType {
	case reflect.TypeOf(DateValue("")):
		return Date, nil
	case reflect.TypeOf(UUIDValue("")):
		return UUID, nil
	case reflect.TypeOf(DecimalValue("")):
		return Decimal, nil
	}
	if timeType := reflect.TypeOf(time.Time{}); fieldType == timeType || fieldType.ConvertibleTo(timeType) {
		return Time, nil
	}
	switch fieldType.Kind() {
	case reflect.String:
		return String, nil
	case reflect.Bool:
		return Bool, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Int, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Uint, nil
	case reflect.Float32, reflect.Float64:
		return Float, nil
	default:
		return 0, fmt.Errorf("Go type %s is not a supported scalar", fieldType)
	}
}

func dereferenceType(fieldType reflect.Type) reflect.Type {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	return fieldType
}

func codecIsNil(codec ScalarCodec) bool {
	if codec == nil {
		return true
	}
	value := reflect.ValueOf(codec)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func resolveGoField(modelType reflect.Type, publicName, explicitName string) (reflect.StructField, error) {
	if explicitName != "" {
		field, ok := modelType.FieldByName(explicitName)
		if !ok || field.PkgPath != "" || len(field.Index) != 1 {
			return reflect.StructField{}, fmt.Errorf("field %q does not select an exported top-level Go field", publicName)
		}
		return field, nil
	}

	var match *reflect.StructField
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if field.PkgPath != "" {
			continue
		}
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "" || jsonName == "-" || jsonName != publicName {
			continue
		}
		if match != nil {
			return reflect.StructField{}, fmt.Errorf("field %q matches more than one json tag", publicName)
		}
		copyOfField := field
		match = &copyOfField
	}
	if match != nil {
		return *match, nil
	}
	field, ok := modelType.FieldByName(publicName)
	if !ok || field.PkgPath != "" || len(field.Index) != 1 {
		return reflect.StructField{}, fmt.Errorf("field %q does not match a json tag or exported Go field", publicName)
	}
	return field, nil
}

func validateGoKind(fieldType reflect.Type, declared Kind) (reflect.Type, int, error) {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	timeType := reflect.TypeOf(time.Time{})
	if declared == Time {
		if fieldType == timeType || fieldType.ConvertibleTo(timeType) {
			return fieldType, 0, nil
		}
		return nil, 0, fmt.Errorf("Go type %s does not match declared type time", fieldType)
	}
	if declared == Date {
		if fieldType == timeType || fieldType.ConvertibleTo(timeType) || fieldType.Kind() == reflect.String {
			return fieldType, 0, nil
		}
		return nil, 0, fmt.Errorf("Go type %s does not match declared type date", fieldType)
	}
	if declared == UUID {
		if fieldType.Kind() == reflect.String || fieldType.Kind() == reflect.Array && fieldType.Len() == 16 && fieldType.Elem().Kind() == reflect.Uint8 {
			return fieldType, 0, nil
		}
		return nil, 0, fmt.Errorf("Go type %s does not match declared type uuid", fieldType)
	}
	if declared == Decimal {
		if fieldType.Kind() == reflect.String {
			return fieldType, 0, nil
		}
		return nil, 0, fmt.Errorf("Go type %s does not match declared type decimal", fieldType)
	}

	matched := false
	switch declared {
	case String:
		matched = fieldType.Kind() == reflect.String
	case Bool:
		matched = fieldType.Kind() == reflect.Bool
	case Int:
		matched = fieldType.Kind() >= reflect.Int && fieldType.Kind() <= reflect.Int64
	case Uint:
		matched = fieldType.Kind() >= reflect.Uint && fieldType.Kind() <= reflect.Uint64
	case Float:
		matched = fieldType.Kind() == reflect.Float32 || fieldType.Kind() == reflect.Float64
	default:
		return nil, 0, fmt.Errorf("unknown declared field type %d", declared)
	}
	if !matched {
		return nil, 0, fmt.Errorf("Go type %s does not match declared type %s", fieldType, declared)
	}
	if declared == Int || declared == Uint || declared == Float {
		return fieldType, fieldType.Bits(), nil
	}
	return fieldType, 0, nil
}

func gormColumn(tag string) string {
	for _, part := range strings.Split(tag, ";") {
		key, value, ok := strings.Cut(part, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "column") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func defaultOperators(kind Kind) []ComparisonOperator {
	switch kind {
	case String:
		return []ComparisonOperator{Eq, Ne, Contains, StartsWith, EndsWith, In, NotIn}
	case Bool:
		return []ComparisonOperator{Eq, Ne, In, NotIn}
	case Int, Uint, Float, Time, Date, Decimal:
		return []ComparisonOperator{Eq, Ne, Gt, Gte, Lt, Lte, In, NotIn}
	case UUID, Custom:
		return []ComparisonOperator{Eq, Ne, In, NotIn}
	default:
		return nil
	}
}

func operatorCompatible(kind Kind, operator ComparisonOperator) bool {
	for _, allowed := range defaultOperators(kind) {
		if operator == allowed {
			return true
		}
	}
	return false
}

func isValidIdentifier(name string) bool {
	if name == "" || !isIdentifierStart(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !isIdentifierContinue(name[i]) {
			return false
		}
	}
	return true
}

func isReservedName(name string) bool {
	if _, reserved := keywordTokens[name]; reserved {
		return true
	}
	return name == "asc" || name == "desc"
}

func schemaError(message, field string, kinds ...Kind) *Error {
	err := &Error{Code: CodeInvalidSchema, Message: message, Field: field}
	if len(kinds) > 0 {
		kind := kinds[0]
		err.Kind = &kind
	}
	return err
}
