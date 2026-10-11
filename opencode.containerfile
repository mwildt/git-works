# Containerfile
FROM docker.io/library/node:24-trixie

RUN npm install -g opencode-ai

RUN mkdir -p /home/node/.local/share/opencode \
             /home/node/.local/state/opencode \
             /home/node/.cache/opencode \
             /home/node/.config/opencode \
    && chown -R node:node /home/node

USER node
WORKDIR /workspace

EXPOSE 4096

ENTRYPOINT ["opencode"]
CMD ["serve", "--hostname", "0.0.0.0", "--port", "4096"]