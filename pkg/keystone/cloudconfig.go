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

import "fmt"

// OpenStackConfig type
type OpenStackConfig struct {
	Clouds struct {
		Default struct {
			Auth struct {
				AuthURL           string `yaml:"auth_url"`
				ProjectName       string `yaml:"project_name"`
				UserName          string `yaml:"username"`
				UserDomainName    string `yaml:"user_domain_name"`
				ProjectDomainName string `yaml:"project_domain_name"`
			} `yaml:"auth"`
			RegionName string `yaml:"region_name"`
		} `yaml:"default"`
	}
}

// OpenStackConfigSecret type
type OpenStackConfigSecret struct {
	Clouds struct {
		Default struct {
			Auth struct {
				Password string `yaml:"password"`
			}
		}
	}
}

// generateCloudrc - generate file contents of a cloudrc file for the clients
// until there is parity with openstackclient.
func GenerateCloudrc(secret *OpenStackConfigSecret, config *OpenStackConfig) string {
	auth := config.Clouds.Default.Auth
	val := fmt.Sprintf(
		"export OS_AUTH_URL=" + auth.AuthURL +
			"\nexport OS_USERNAME=" + auth.UserName +
			"\nexport OS_PROJECT_NAME=" + auth.ProjectName +
			"\nexport OS_PROJECT_DOMAIN_NAME=" + auth.ProjectDomainName +
			"\nexport OS_USER_DOMAIN_NAME=" + auth.UserDomainName +
			"\nexport OS_PASSWORD=" + secret.Clouds.Default.Auth.Password)
	return val
}
