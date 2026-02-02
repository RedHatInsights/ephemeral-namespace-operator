package helpers

// CustomAnnotation represents a key-value pair for Kubernetes annotations
type CustomAnnotation struct {
	Annotation string
	Value      string
}

// CustomLabel represents a key-value pair for Kubernetes labels
type CustomLabel struct {
	Label string
	Value string
}

// CreateInitialAnnotations returns the default annotations for a new ephemeral namespace
func CreateInitialAnnotations() map[string]string {
	return map[string]string{
		AnnotationEnvStatus: EnvStatusCreating,
		AnnotationReserved:  FalseValue,
	}
}

// CreateInitialLabels returns the default labels for a new ephemeral namespace belonging to a pool
func CreateInitialLabels(poolName string) map[string]string {
	return map[string]string{
		LabelOperatorNS: TrueValue,
		LabelPool:       poolName,
	}
}

// ToMap converts a CustomAnnotation to a map representation
func (a *CustomAnnotation) ToMap() map[string]string {
	return map[string]string{a.Annotation: a.Value}
}

// ToMap converts a CustomLabel to a map representation
func (l *CustomLabel) ToMap() map[string]string {
	return map[string]string{l.Label: l.Value}
}

// AnnotationEnvReady represents the annotation indicating an environment is ready
var AnnotationEnvReady = CustomAnnotation{Annotation: AnnotationEnvStatus, Value: EnvStatusReady}

// AnnotationEnvCreating represents the annotation indicating an environment is being created
var AnnotationEnvCreating = CustomAnnotation{Annotation: AnnotationEnvStatus, Value: EnvStatusCreating}

// AnnotationEnvError represents the annotation indicating an environment encountered an error
var AnnotationEnvError = CustomAnnotation{Annotation: AnnotationEnvStatus, Value: EnvStatusError}

// AnnotationEnvDeleting represents the annotation indicating an environment is being deleted
var AnnotationEnvDeleting = CustomAnnotation{Annotation: AnnotationEnvStatus, Value: EnvStatusDeleting}

// AnnotationReservedTrue represents the annotation indicating a namespace is reserved
var AnnotationReservedTrue = CustomAnnotation{Annotation: AnnotationReserved, Value: TrueValue}

// AnnotationReservedFalse represents the annotation indicating a namespace is not reserved
var AnnotationReservedFalse = CustomAnnotation{Annotation: AnnotationReserved, Value: FalseValue}

// LabelOperatorNamespaceTrue represents the label marking a namespace as managed by this operator
var LabelOperatorNamespaceTrue = CustomLabel{Label: LabelOperatorNS, Value: TrueValue}

// ErrType is used as a key for storing typed values in context
type ErrType string
