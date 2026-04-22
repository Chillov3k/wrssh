FROM nginx:alpine

COPY docker/frontend/default.conf.template /etc/nginx/templates/default.conf.template
COPY docker/frontend/entrypoint.sh /app/entrypoint.sh
COPY frontend/*.html /usr/share/nginx/html/
COPY frontend/*.js /usr/share/nginx/html/
COPY frontend/*.png /usr/share/nginx/html/
COPY frontend/styles.css /usr/share/nginx/html/styles.css

RUN chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]
