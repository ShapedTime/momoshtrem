import '@testing-library/jest-dom';
import React from 'react';

// Mock next/navigation
jest.mock('next/navigation', () => ({
  useRouter: () => ({
    push: jest.fn(),
    replace: jest.fn(),
    back: jest.fn(),
    forward: jest.fn(),
    refresh: jest.fn(),
    prefetch: jest.fn(),
  }),
  useSearchParams: () => new URLSearchParams(),
  usePathname: () => '/',
}));

// Mock next/image - using createElement instead of JSX
jest.mock('next/image', () => ({
  __esModule: true,
  default: function MockImage(props: React.ComponentProps<'img'> & { priority?: boolean; fill?: boolean }) {
    const { priority, fill, ...imgProps } = props;
    return React.createElement('img', {
      ...imgProps,
      'data-priority': priority ? 'true' : undefined,
      fetchPriority: priority ? 'high' : undefined,
    });
  },
}));

// jsdom gaps that Radix, Vaul and the media-query hook rely on (skipped in node-environment tests)
if (typeof window !== 'undefined') {
  if (!window.matchMedia) {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: (query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      }),
    });
  }

  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  if (!('ResizeObserver' in window)) {
    Object.defineProperty(window, 'ResizeObserver', { writable: true, value: ResizeObserverStub });
  }

  Element.prototype.scrollIntoView ??= function scrollIntoView() {};
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.releasePointerCapture ??= () => {};
  Element.prototype.setPointerCapture ??= () => {};
}
