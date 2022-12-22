package helpers

const (
	AnnotationEnvStatus = "env-status"
	AnnotationReserved  = "reserved"

	CompletionTime = "completion-time"

	EnvStatusCreating = "creating"
	EnvStatusDeleting = "deleting"
	EnvStatusError    = "error"
	EnvStatusReady    = "ready"

	KindNamespacePool = "NamespacePool"

	LabelOperatorNS = "operator-ns"
	LabelPool       = "pool"

	NamespaceEphemeralBase = "ephemeral-base"

	BonfireGinoreSecret         = "bonfire.ignore"
	OpenShiftVaultSecretsSecret = "openshift-vault-secrets"
	QontractIntegrationSecret   = "qontract.integration"

	TrueValue  = "true"
	FalseValue = "false"
)
