package helpers

type CustomAnnotation struct {
	Annotation string
	Value      string
}

type CustomLabel struct {
	Label string
	Value string
}

func (c *CustomAnnotation) ToMap() map[string]string {
	return map[string]string{c.Annotation: c.Value}
}

func (c *CustomLabel) ToMap() map[string]string {
	return map[string]string{c.Label: c.Value}
}

var AnnotationEnvReady = CustomAnnotation{Annotation: ANNOTATION_ENV_STATUS, Value: ENV_STATUS_READY}
var AnnotationEnvCreating = CustomAnnotation{Annotation: ANNOTATION_ENV_STATUS, Value: ENV_STATUS_CREATING}
var AnnotationEnvError = CustomAnnotation{Annotation: ANNOTATION_ENV_STATUS, Value: ENV_STATUS_ERROR}

var AnnotationReservedTrue = CustomAnnotation{Annotation: ANNOTATION_RESERVED, Value: TRUE_VALUE}
var AnnotationReservedFalse = CustomAnnotation{Annotation: ANNOTATION_RESERVED, Value: FALSE_VALUE}

var LabelPoolType = CustomLabel{Label: LABEL_POOL, Value: ""}
