set -e

# This script generates the keystone-passwords.conf file and copies the result
# to the ephemeral /var/lib/emptydir volume (mounted by your init container).
# 
# Secrets are obtained from ENV variables.
export DatabasePassword=${DatabasePassword:?"Please specify a DatabasePassword variable."}
export AdminPassword=${AdminPassword:?"Please specify a AdminPassword variable."}
export DatabaseHost=${DatabaseHost:?"Please specify a DatabaseHost variable."}
export DatabaseUser=${DatabaseUser:-"keystone"}
export DatabaseSchema=${DatabaseSchema:-"keystone"}

cat > /var/lib/emptydir/keystone-passwords.conf <<-EOF_CAT
[DEFAULT]
admin_token=$AdminPassword

[database]
connection=mysql+pymysql://$DatabaseUser:$DatabasePassword@$DatabaseHost/$DatabaseSchema
EOF_CAT
