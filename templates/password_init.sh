set -e

# This script generates the keystone.conf file and copies the result
# to the ephemeral /var/lib/emptydir volume (mounted by your init container).
# 
# Secrets are obtained from ENV variables.
export DatabasePassword=${DatabasePassword:?"Please specify a DatabasePassword variable."}
export AdminPassword=${AdminPassword:?"Please specify a AdminPassword variable."}
export DatabaseHost=${DatabaseHost:?"Please specify a DatabaseHost variable."}
export DatabaseUser=${DatabaseUser:-"keystone"}
export DatabaseSchema=${DatabaseSchema:-"keystone"}

cat > /var/lib/emptydir/keystone.conf <<-EOF_CAT
[DEFAULT]
admin_token=$AdminPassword
log_config_append=/etc/keystone/logging.conf

[catalog]
template_file=/etc/keystone/default_catalog.templates

[database]
max_retries=-1
db_max_retries=-1
connection=mysql+pymysql://$DatabaseUser:$DatabasePassword@$DatabaseHost/$DatabaseSchema

[fernet_tokens]
key_repository=/etc/keystone/fernet-keys
max_active_keys=2
EOF_CAT
