package helpers

func IsPoolAtLimit(currentSize int, sizeLimit int) bool {
	return currentSize == sizeLimit
}

func CalculateNamespaceQuantityDelta(poolSizeLimit *int, size int, namespaceReady int, namespaceCreating int, namespaceReserved int) int {
	currentNamespaceQuantity := namespaceReady + namespaceCreating + namespaceReserved
	currentNamespaceQueue := namespaceReady + namespaceCreating

	if poolSizeLimit == nil {
		return size - currentNamespaceQueue
	}

	sizeLimit := *poolSizeLimit

	if sizeLimit-currentNamespaceQuantity < size-currentNamespaceQueue {
		return sizeLimit - currentNamespaceQuantity
	}

	return size - currentNamespaceQueue
}
