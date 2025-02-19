FROM golang:1.19.4

#ARG UID
#ARG GID
#RUN sudo groupadd -g $GID -o user
#RUN sudo useradd -m -u $UID -g $GID -o -s /bin/bash user

# expected to mount repo root as /workdir/repo
# docker run --mount type=bind,src=<host-path>,dst=/usr/src/app

RUN apt update
RUN apt install -y upx squashfs-tools && apt-get clean

#USER user
#ADD build.sh .

WORKDIR /usr/src/app

CMD ["./build.sh"]
