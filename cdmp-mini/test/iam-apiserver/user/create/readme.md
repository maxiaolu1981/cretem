python3 check_user_insert_loss.py \
  --db-host localhost \
  --db-user iam \
  --db-fallback-host 192.168.10.8 \
  --redis-host 192.168.10.14,192.168.10.8 \
  --redis-port 6379,6380,6381 \
  --redis-pattern genericapiserver:user* \
  --success-file ../success_usernames.txt \
  --output-dir ./result \
  --db-dump ./result/db_users.txt \
  --missing-file ./result/missing.txt \
  --sample-missing-file ./result/missing_sample.txt \
  --log-script ./find_missing_user_log.py
  