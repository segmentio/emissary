FROM 528451384384.dkr.ecr.us-west-2.amazonaws.com/segment-ubuntu
COPY ./emissary /emissary
CMD [ "/emissary", "eds", "-consul", "consul:8500" ]
