
  mysql -h 127.0.0.1 -u root -p'iam59!z$' -D iam \
  -e "SELECT name FROM \`user\`;" \
  | tail -n +2 > /home/mxl/cretem/cretem/cdmp-mini/test/iam-apiserver/user/delete-force/output/db_baseline_usernames.txt

  