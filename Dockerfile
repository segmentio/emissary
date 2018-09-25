FROM scratch

COPY ./emissary /emissary

ENTRYPOINT ["/emissary"]
CMD [ "eds", "-consul", "consul:8500" ]
