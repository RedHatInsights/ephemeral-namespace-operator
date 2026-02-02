package helpers

const (
	// AnnotationEnvStatus tracks the current status of an ephemeral namespace environment
	AnnotationEnvStatus = "env-status"
	// AnnotationReserved indicates whether a namespace has been reserved by a user
	AnnotationReserved = "reserved"

	// CompletionTime tracks when a namespace completed its provisioning
	CompletionTime = "completion-time"

	// EnvStatusCreating indicates an environment is being created
	EnvStatusCreating = "creating"
	// EnvStatusDeleting indicates an environment is being deleted
	EnvStatusDeleting = "deleting"
	// EnvStatusError indicates an environment encountered an error
	EnvStatusError = "error"
	// EnvStatusReady indicates an environment is ready for use
	EnvStatusReady = "ready"

	// KindNamespacePool is the Kind name for NamespacePool resources
	KindNamespacePool = "NamespacePool"

	// LabelOperatorNS labels namespaces managed by this operator
	LabelOperatorNS = "operator-ns"
	// LabelPool identifies which pool a namespace belongs to
	LabelPool = "pool"

	// NamespaceEphemeralBase is the source namespace for copying secrets
	NamespaceEphemeralBase = "ephemeral-base"
	// NamespaceHcmAi is the namespace for AI development team secrets
	NamespaceHcmAi = "ephemeral-hcm-ai"

	// PoolAiDevelopment is the name of the AI development pool
	PoolAiDevelopment = "ai-development"

	// BonfireIgnoreAnnotation marks secrets that should not be copied
	BonfireIgnoreAnnotation = "bonfire.ignore"
	// OpenShiftVaultSecretsProvider identifies vault-managed secrets
	OpenShiftVaultSecretsProvider = "openshift-vault-secrets"
	// OpenShiftRhcsCertsProvider identifies RHCS certificate secrets
	OpenShiftRhcsCertsProvider = "openshift-rhcs-certs"
	// QontractIntegrationAnnotation identifies app-interface managed secrets
	QontractIntegrationAnnotation = "qontract.integration"

	// TrueValue is the string representation of true for annotations/labels
	TrueValue = "true"
	// FalseValue is the string representation of false for annotations/labels
	FalseValue = "false"
)
