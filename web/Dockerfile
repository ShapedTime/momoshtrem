# ===============
# Stage 1: Dependencies
# ===============
FROM node:20-alpine AS deps

WORKDIR /app

# Install dependencies only when package files change
COPY package.json package-lock.json* ./
RUN npm ci

# ===============
# Stage 2: Build
# ===============
FROM node:20-alpine AS builder

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

# Build the Next.js application
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# ===============
# Stage 3: Production
# ===============
FROM node:20-alpine AS runner

# Install curl for Docker health checks
RUN apk add --no-cache curl

WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

# Create non-root user for security
RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

# Copy built application
COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000

ENV PORT=3000
ENV HOSTNAME="0.0.0.0"

CMD ["node", "server.js"]
