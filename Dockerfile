FROM scratch
COPY ./emissary /emissary
CMD [ "/emissary", "eds", "-consul", "consul:8500" ]
