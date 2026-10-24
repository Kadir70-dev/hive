# Backup/restore toolbox: pg_dump/pg_restore matching the server major version,
# age for public-key encryption, rclone for optional off-site copies.
FROM postgres:17-alpine
RUN apk add --no-cache age rclone bash coreutils
COPY backup.sh restore.sh verify.sh entrypoint.sh /usr/local/bin/
RUN chmod 0755 /usr/local/bin/backup.sh /usr/local/bin/restore.sh /usr/local/bin/verify.sh /usr/local/bin/entrypoint.sh
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["cron"]
