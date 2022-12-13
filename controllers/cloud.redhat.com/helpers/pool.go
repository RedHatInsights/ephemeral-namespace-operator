package helpers

func IsPoolAtLimit(currentSize int, sizeLimit int) bool {
	return currentSize == sizeLimit
}

func CalculateNamespaceQuantityDelta(poolSizeLimit *int, size int, namespacesReady int, namespacesCreating int, namespacesReserved int) int {
	currentNamespaceQuantity := namespacesReady + namespacesCreating + namespacesReserved
	currentNamespaceQueue := namespacesReady + namespacesCreating

	if poolSizeLimit == nil {
		return size - currentNamespaceQueue
	}

	sizeLimit := *poolSizeLimit

	if sizeLimit-currentNamespaceQuantity < size-currentNamespaceQueue {
		return sizeLimit - currentNamespaceQuantity
	}

	return size - currentNamespaceQueue
}
