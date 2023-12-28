package ksink

import "math/rand"

func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz" +
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}

	return string(b)
}

func I32Ptr(i int32) *int32 {
	return &i
}

func I64Ptr(i int64) *int64 {
	return &i
}
