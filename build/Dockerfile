FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

LABEL vendor="Open Liberty" \
      name="Open Liberty Operator" \
      version="0.7.1" \
      summary="Image for Open Liberty Operator" \
      description="This image contains the controllers for Open Liberty Operator. See https://github.com/OpenLiberty/open-liberty-operator#open-liberty-operator"
      

ENV OPERATOR=/usr/local/bin/open-liberty-operator \
    USER_UID=1001 \
    USER_NAME=open-liberty-operator

# install operator binary
COPY build/_output/bin/open-liberty-operator ${OPERATOR}
COPY build/bin /usr/local/bin
COPY LICENSE /licenses/

RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
