package util

import (
	"fmt"
	"hash/fnv"
	"k8s.io/apimachinery/pkg/util/rand"
	hashutil "k8s.io/kubernetes/pkg/util/hash"
)

// ObjectHash creates a deep object hash and return it as a safe encoded string
func ObjectHash(i interface{}) string {
	hf := fnv.New32()
	hashutil.DeepHashObject(hf, i)
	return rand.SafeEncodeString(fmt.Sprint(hf.Sum32()))
}
