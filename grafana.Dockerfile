FROM node:18-alpine as plugin-builder

# Install dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /plugins

# Copy and install dependencies for vechain-heatmap-panel
WORKDIR /plugins/vechain-heatmap-panel
COPY plugins/vechain-heatmap-panel/package.json .
RUN npm install

# Copy and install dependencies for vechain-slotmap-panel
WORKDIR /plugins/vechain-slotmap-panel
COPY plugins/vechain-slotmap-panel/package.json .
RUN npm install

# Copy remaining files for vechain-heatmap-panel
WORKDIR /plugins/vechain-heatmap-panel
COPY plugins/vechain-heatmap-panel/ .

# Copy remaining files for vechain-slotmap-panel
WORKDIR /plugins/vechain-slotmap-panel
COPY plugins/vechain-slotmap-panel/ .

# Build vechain-heatmap-panel
WORKDIR /plugins/vechain-heatmap-panel
RUN npm run build

# Build vechain-slotmap-panel
WORKDIR /plugins/vechain-slotmap-panel
RUN npm run build

ENTRYPOINT [ "tail", "-f", "/dev/null" ]
