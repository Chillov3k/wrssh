FROM nginx:alpine

COPY docker/frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY frontend/*.html /usr/share/nginx/html/
COPY frontend/*.js /usr/share/nginx/html/
COPY frontend/*.png /usr/share/nginx/html/
COPY frontend/styles.css /usr/share/nginx/html/styles.css
