FROM scratch
COPY ./emissary /emissary
ENTRYPOINT ["/emissary"]
