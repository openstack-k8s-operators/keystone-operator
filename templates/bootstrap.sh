kolla_set_configs && keystone-manage --config-file=/etc/keystone/keystone.conf \
bootstrap --bootstrap-password {{.AdminPassword}} \
--bootstrap-service-name keystone \
--bootstrap-project-name admin \
--bootstrap-role-name admin \
--bootstrap-admin-url {{.ApiEndpoint}} \
--bootstrap-public-url {{.ApiEndpoint}} \
--bootstrap-internal-url http://{{.ServiceName}}/ #FIXME: this should also support TLS
