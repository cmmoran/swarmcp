FROM python:3.12-slim

WORKDIR /site

RUN python3 -m pip install --no-cache-dir \
  "mkdocs>=1.6,<2.0" \
  "mkdocs-include-markdown-plugin>=7.1,<8.0"

ENTRYPOINT ["mkdocs"]
