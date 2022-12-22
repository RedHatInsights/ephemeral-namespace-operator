package helpers

type CustomAnnotation struct {
	Annotation string
	Value      string
}

type CustomLabel struct {
	Label string
	Value string
}

func CreateInitialAnnotations() map[string]string {
	return map[string]string{
		AnnotationEnvStatus: EnvStatusCreating,
		AnnotationReserved:  FalseValue,
	}
}

func CreateInitialLabels(poolName string) map[string]string {
	return map[string]string{
		LabelOperatorNS: TrueValue,
		LabelPool:       poolName,
	}
}

func (a *CustomAnnotation) ToMap() map[string]string {
	return map[string]string{a.Annotation: a.Value}
}

func (l *CustomLabel) ToMap() map[string]string {
	return map[string]string{l.Label: l.Value}
}

var AnnotationEnvReady = CustomAnnotation{Annotation: AnnotationEnvStatus, Value: EnvStatusReady}
var AnnotationEnvCreating = CustomAnnotation{Annotation: AnnotationEnvStatus, Value: EnvStatusCreating}
var AnnotationEnvError = CustomAnnotation{Annotation: AnnotationEnvStatus, Value: EnvStatusError}
var AnnotationEnvDeleting = CustomAnnotation{Annotation: AnnotationEnvStatus, Value: EnvStatusDeleting}

var AnnotationReservedTrue = CustomAnnotation{Annotation: AnnotationReserved, Value: TrueValue}
var AnnotationReservedFalse = CustomAnnotation{Annotation: AnnotationReserved, Value: FalseValue}

var LabelOperatorNamespaceTrue = CustomLabel{Label: LabelOperatorNS, Value: TrueValue}
