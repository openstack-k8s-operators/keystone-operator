/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package keystone

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/go-logr/logr"
)

// GenerateFernetKey - returns a base64-encoded, 32-byte key using cryptographically secure random generation
func GenerateFernetKey(logger logr.Logger) string {
	data := make([]byte, 32)
	_, err := rand.Read(data)
	if err != nil {
		logger.Error(err, "failed to read random bytes for Fernet key generation")
		return ""
	}

	return base64.StdEncoding.EncodeToString(data)
}
