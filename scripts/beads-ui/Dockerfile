# beads-ui - Web dashboard for beads issue tracker
# Connects to Dolt SQL server via Docker network
#
# Build args:
#   BEADS_UI_VERSION - beads-ui npm package version (default: 0.11.1)

FROM node:22-slim

ARG BEADS_UI_VERSION=0.11.1

# Install bd CLI and beads-ui via npm
RUN npm install -g @beads/bd beads-ui@${BEADS_UI_VERSION}

# Create app directory owned by node user
RUN mkdir -p /app/.beads && chown -R node:node /app

WORKDIR /app
USER node

ENV BD_BIN=/usr/local/bin/bd
# bd installed via npm lands in /usr/local/bin/ (node global bin)

EXPOSE 9080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=15s \
  CMD node -e "fetch('http://localhost:'+(process.env.PORT||9080)+'/').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"

CMD ["sh", "-c", "exec node /usr/local/lib/node_modules/beads-ui/server/index.js"]