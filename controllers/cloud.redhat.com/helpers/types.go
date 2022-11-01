package helpers

import (
	core "k8s.io/api/core/v1"
)

type CustomAnnotation struct {
	Annotation string
	Value      string
}

type CustomLabel struct {
	Label string
	Value string
}

func CreateInitialAnnotations() map[string]string {
	initialAnnotations := make(map[string]string)

	initialAnnotations[ANNOTATION_ENV_STATUS] = ENV_STATUS_CREATING
	initialAnnotations[ANNOTATION_RESERVED] = FALSE_VALUE

	return initialAnnotations
}

func CreateInitialLabels(poolName string) map[string]string {
	initialLabels := make(map[string]string)

	initialLabels[LABEL_OPERATOR_NS] = TRUE_VALUE
	initialLabels[LABEL_POOL] = poolName

	return initialLabels
}

func (a *CustomAnnotation) ToMap() map[string]string {
	return map[string]string{a.Annotation: a.Value}
}

func (a *CustomAnnotation) SetInitialAnnotations(ns *core.Namespace) {
	initialAnnotations := CreateInitialAnnotations()

	if len(ns.Annotations) == 0 {
		ns.SetAnnotations(initialAnnotations)
	} else {
		for k, v := range initialAnnotations {
			ns.Annotations[k] = v
		}
	}
}

func (l *CustomLabel) SetInitialLabels(ns *core.Namespace, poolName string) {
	initialLabels := CreateInitialLabels(poolName)

	if len(ns.Labels) == 0 {
		ns.SetLabels(initialLabels)
	} else {
		for k, v := range initialLabels {
			ns.Labels[k] = v
		}
	}
}

var AnnotationEnvReady = CustomAnnotation{Annotation: ANNOTATION_ENV_STATUS, Value: ENV_STATUS_READY}
var AnnotationEnvCreating = CustomAnnotation{Annotation: ANNOTATION_ENV_STATUS, Value: ENV_STATUS_CREATING}
var AnnotationEnvError = CustomAnnotation{Annotation: ANNOTATION_ENV_STATUS, Value: ENV_STATUS_ERROR}
var AnnotationEnvDeleting = CustomAnnotation{Annotation: ANNOTATION_ENV_STATUS, Value: ENV_STATUS_DELETING}

var AnnotationReservedTrue = CustomAnnotation{Annotation: ANNOTATION_RESERVED, Value: TRUE_VALUE}
var AnnotationReservedFalse = CustomAnnotation{Annotation: ANNOTATION_RESERVED, Value: FALSE_VALUE}

var LabelPoolType = CustomLabel{Label: LABEL_POOL, Value: ""}
var LabelOperatorNamespaceTrue = CustomLabel{Label: LABEL_OPERATOR_NS, Value: TRUE_VALUE}
