# beads-ui - Web dashboard for beads issue tracker
# Connects to Dolt SQL server via Docker network
#
# Build args:
#   BEADS_UI_VERSION - beads-ui npm package version (default: 0.11.1)

FROM node:22-slim

ARG BEADS_UI_VERSION=0.11.1

# Install dependencies for bd CLI installer
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install bd CLI (beads issue tracker)
RUN curl -fsSL https://beads.sh | bash \
    && mv /root/.local/bin/bd /usr/local/bin/bd \
    && chmod +x /usr/local/bin/bd

# Install beads-ui globally
RUN npm install -g beads-ui@${BEADS_UI_VERSION}

# Create app directory owned by node user
RUN mkdir -p /app/.beads && chown -R node:node /app

WORKDIR /app
USER node

ENV BD_BIN=/usr/local/bin/bd

EXPOSE 9080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=15s \
  CMD node -e "fetch('http://localhost:'+(process.env.PORT||9080)+'/').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"

CMD ["sh", "-c", "exec bdui start --host 0.0.0.0 --port ${PORT:-9080}"]