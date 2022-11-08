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
		ANNOTATION_ENV_STATUS: ENV_STATUS_CREATING,
		ANNOTATION_RESERVED:   FALSE_VALUE,
	}
}

func CreateInitialLabels(poolName string) map[string]string {
	return map[string]string{
		LABEL_OPERATOR_NS: TRUE_VALUE,
		LABEL_POOL:        poolName,
	}
}

func (a *CustomAnnotation) ToMap() map[string]string {
	return map[string]string{a.Annotation: a.Value}
}

func (l *CustomLabel) ToMap() map[string]string {
	return map[string]string{l.Label: l.Value}
}

var AnnotationEnvReady = CustomAnnotation{Annotation: ANNOTATION_ENV_STATUS, Value: ENV_STATUS_READY}
var AnnotationEnvCreating = CustomAnnotation{Annotation: ANNOTATION_ENV_STATUS, Value: ENV_STATUS_CREATING}
var AnnotationEnvError = CustomAnnotation{Annotation: ANNOTATION_ENV_STATUS, Value: ENV_STATUS_ERROR}
var AnnotationEnvDeleting = CustomAnnotation{Annotation: ANNOTATION_ENV_STATUS, Value: ENV_STATUS_DELETING}

var AnnotationReservedTrue = CustomAnnotation{Annotation: ANNOTATION_RESERVED, Value: TRUE_VALUE}
var AnnotationReservedFalse = CustomAnnotation{Annotation: ANNOTATION_RESERVED, Value: FALSE_VALUE}

var LabelOperatorNamespaceTrue = CustomLabel{Label: LABEL_OPERATOR_NS, Value: TRUE_VALUE}
