kolla_set_configs && keystone-manage --config-file=/etc/keystone/keystone.conf \
bootstrap --bootstrap-password $AdminPassword \
--bootstrap-service-name keystone \
--bootstrap-project-name admin \
--bootstrap-role-name admin \
--bootstrap-admin-url {{.APIEndpoint}} \
--bootstrap-public-url {{.APIEndpoint}} \
--bootstrap-internal-url http://{{.ServiceName}}/  \
--bootstrap-region-id regionOne
