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
	NamespaceHcmAi         = "ephemeral-hcm-ai"

	PoolAiDevelopment = "ai-development"

	BonfireIgnoreAnnotation       = "bonfire.ignore"
	OpenShiftVaultSecretsProvider = "openshift-vault-secrets"
	OpenShiftRhcsCertsProvider    = "openshift-rhcs-certs"
	QontractIntegrationAnnotation = "qontract.integration"

	TrueValue  = "true"
	FalseValue = "false"
)
