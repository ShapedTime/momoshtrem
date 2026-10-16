import type { Config } from 'jest';
import nextJest from 'next/jest';

const createJestConfig = nextJest({
  // Provide the path to your Next.js app to load next.config.js and .env files
  dir: './',
});

const config: Config = {
  coverageProvider: 'v8',
  testEnvironment: 'jsdom',
  setupFilesAfterEnv: ['<rootDir>/jest.setup.ts'],
  moduleNameMapper: {
    '^@/(.*)$': '<rootDir>/src/$1',
  },
  testPathIgnorePatterns: ['<rootDir>/node_modules/', '<rootDir>/.next/', '<rootDir>/e2e/'],
  modulePathIgnorePatterns: ['<rootDir>/.next/', '<rootDir>/.next-e2e/'],
  collectCoverageFrom: [
    'src/**/*.{ts,tsx}',
    '!src/**/*.d.ts',
    '!src/app/**/layout.tsx',
    '!src/app/**/page.tsx',
  ],
};

// ESM-only packages that must go through the SWC transform. next/jest sets
// transformIgnorePatterns itself, so override it after the config is built.
const ESM_PACKAGES = ['jose', 'msw', '@mswjs', '@open-draft', 'rettime', 'until-async', 'outvariant', 'strict-event-emitter', 'headers-polyfill', 'is-node-process', '@bundled-es-modules'];
// nuqs: Discover's URL state (#100)
ESM_PACKAGES.push('nuqs');

export default async function jestConfig(): Promise<Config> {
  const resolved = await createJestConfig(config)();
  return {
    ...resolved,
    transformIgnorePatterns: [
      `/node_modules/(?!(${ESM_PACKAGES.join('|')})/)`,
      '^.+\\.module\\.(css|sass|scss)$',
    ],
  };
}
