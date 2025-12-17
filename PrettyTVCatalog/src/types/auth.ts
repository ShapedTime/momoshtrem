export interface AuthState {
  isAuthenticated: boolean;
  error?: string;
}

export interface LoginRequest {
  password: string;
}

export interface LoginResponse {
  success: boolean;
  error?: string;
}

export interface SessionPayload {
  authenticated: boolean;
  expiresAt: number;
}
