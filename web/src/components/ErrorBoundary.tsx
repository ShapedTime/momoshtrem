'use client';

import { Component, type ErrorInfo, type ReactNode } from 'react';
import { ErrorView } from '@/components/system/ErrorView';

interface ErrorBoundaryProps {
  children: ReactNode;
  fallback?: ReactNode;
}

interface ErrorBoundaryState {
  error: Error | null;
}

/** Catches render errors below it and shows the calm error view instead of a blank page. */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error('ErrorBoundary caught an error:', error, info);
  }

  render(): ReactNode {
    if (!this.state.error) return this.props.children;
    return this.props.fallback ?? <ErrorView onRetry={() => this.setState({ error: null })} />;
  }
}
