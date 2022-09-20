package helpers

const (
	ANNOTATION_ENV_STATUS = "env-status"
	ANNOTATION_RESERVED   = "reserved"

	COMPLETION_TIME = "completion-time"

	ENV_STATUS_CREATING = "creating"
	ENV_STATUS_DELETING = "deleting"
	ENV_STATUS_ERROR    = "error"
	ENV_STATUS_READY    = "ready"

	KIND_NAMESPACEPOOL = "NamespacePool"

	LABEL_POOL        = "pool"
	LABEL_OPERATOR_NS = "operator-ns"

	NAMESPACE_EPHEMERAL_BASE = "ephemeral-base"

	SECRET_BONFIRE_IGNORE          = "bonfire.ignore"
	SECRET_OPENSHIFT_VAULT_SECRETS = "openshift-vault-secrets"
	SECRET_QONTRACT_INTEGRATION    = "qontract.integration"
)
