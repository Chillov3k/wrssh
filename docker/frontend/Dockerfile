FROM nginx:alpine

COPY docker/frontend/*.template /etc/nginx/templates/
COPY docker/frontend/entrypoint.sh /app/entrypoint.sh
COPY frontend/*.html /usr/share/nginx/html/
COPY frontend/*.js /usr/share/nginx/html/
COPY frontend/*.png /usr/share/nginx/html/
COPY frontend/styles.css /usr/share/nginx/html/styles.css

RUN chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]
