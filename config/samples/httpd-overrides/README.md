# Keystone HTTPD Configuration Overrides

The keystone-operator provides mechanisms to customize the Apache HTTPD server
configuration through the use of custom configuration files. This feature
leverages the
[ExtraMounts](https://github.com/openstack-k8s-operators/dev-docs/blob/main/extra_mounts.md)
functionality to mount custom HTTPD configuration files into the Keystone
deployment.

## How It Works

1. **Custom Configuration Files**: Create HTTPD configuration files with your
   custom settings
2. **ConfigMap**: Create ConfigMaps from files containing the overrides
3. **OpenStackControlPlane Patch**: Patch the control plane to mount the
   generated ConfigMap into Keystone containers. The HTTPD configuration
   automatically includes files mounted to `/etc/httpd/conf_custom/*.conf`


### Step 1: Create Custom HTTPD Configuration

Create your custom HTTPD configuration file(s). As a best practice the filename
could start with the `httpd_custom_` prefix, but all `*.conf` files mounted to
`/etc/httpd/conf_custom/` are automatically included by the `IncludeOptional`
directive in the base `httpd` configuration.

Example (`httpd_custom_timeout.conf`):
```apache
# Custom timeout settings for Keystone
Timeout 300
KeepAliveTimeout 15
```

### Step 2. Create a ConfigMap

Create a Kubernetes `ConfigMap` containing your custom configuration files:

```bash
oc create configmap httpd-overrides --from-file=httpd_custom_timeout.conf
```

It is possible to add multiple configuration files containing dedicated
configuration directives:

```bash
oc create configmap httpd-overrides \
  --from-file=httpd_custom_timeout.conf \
  --from-file=httpd_custom_security.conf \
  --from-file=httpd_custom_logging.conf
```

The following example is based on a single customization file and demonstrates
how to set custom `Timeout` and `KeepAliveTimeout` parameters.

### Step 3: Configure ExtraMounts in the OpenStackControlPlane

Update your `OpenStackControlPlane` resource to include the custom HTTPD
configuration files using `extraMounts`. The simplest approach is to mount
the entire ConfigMap to the target `/etc/httpd/conf_custom` mount point:

```yaml
apiVersion: core.openstack.org/v1beta1
kind: OpenStackControlPlane
metadata:
  name: openstack
spec:
  keystone:
    template:
      extraMounts:
      - extraVol:
        - extraVolType: httpd-overrides
          mounts:
          - mountPath: /etc/httpd/conf_custom
            name: httpd-overrides
            readOnly: true
          volumes:
          - configMap:
              name: httpd-overrides
            name: httpd-overrides
```

## Common Use Cases

- **Timeout Adjustments**: Modify request timeout values for specific environments
- **Security Headers**: Add custom security headers or configurations
- **Logging**: Customize Apache logging configuration
- **Performance Tuning**: Adjust worker processes, connection limits, etc.

## Verification

After deploying your custom `HTTPD` configuration, you can verify that the
settings have been properly applied:

### 1. Find the Keystone Pod

First, identify the running Keystone pod:

```bash
$ oc get pods -l service=keystone
```

### 2. Verify Configuration Loading

Connect to the Keystone Pod and check that your custom configuration has been
loaded:

```bash
# Replace <keystone-pod-name> with the actual pod name from step 1
oc rsh -c keystone-api <keystone-pod-name>
# Inside the pod, dump the HTTPD configuration and check for your custom settings
httpd -D DUMP_CONFIG
```

### 3. Additional Verification Commands

You can also verify other aspects of the configuration:

```bash
# Check all loaded configuration files
$ httpd -D DUMP_INCLUDES
```
