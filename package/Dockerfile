FROM ubuntu:16.04
COPY deploy.sh /usr/bin/
COPY local-flexvolume /usr/bin/
RUN cp /usr/bin/local-flexvolume /localflexvolume
CMD ["deploy.sh"]
