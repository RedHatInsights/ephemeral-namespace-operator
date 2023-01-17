package controllers

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("", func() {
	const (
		timeout  = time.Second * 35
		interval = time.Millisecond * 30
	)

	Context("", func() {
		Eventually(func() bool {
			return true
		}, timeout, interval).Should(BeTrue())
	})
})
