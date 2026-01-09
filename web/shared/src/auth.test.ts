import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { getUserEmail, setUserEmail, clearUserEmail, isAuthenticated } from './auth';

describe('auth utilities', () => {
  const originalDocument = global.document;

  beforeEach(() => {
    // Mock document.cookie
    let cookieStore = '';
    Object.defineProperty(global.document, 'cookie', {
      get: vi.fn(() => cookieStore),
      set: vi.fn((value: string) => {
        // Simple cookie parser for testing
        if (value.includes('expires=Thu, 01 Jan 1970')) {
          // Clear cookie
          cookieStore = '';
        } else {
          // Set cookie
          const [cookiePart] = value.split(';');
          cookieStore = cookiePart;
        }
      }),
      configurable: true,
    });
  });

  afterEach(() => {
    // Clean up
    if (originalDocument) {
      Object.defineProperty(global, 'document', {
        value: originalDocument,
        configurable: true,
      });
    }
  });

  it('getUserEmail returns null when no cookie is set', () => {
    expect(getUserEmail()).toBeNull();
  });

  it('setUserEmail stores email in cookie', () => {
    setUserEmail('test@example.com');
    expect(getUserEmail()).toBe('test@example.com');
  });

  it('clearUserEmail removes email from cookie', () => {
    setUserEmail('test@example.com');
    expect(getUserEmail()).toBe('test@example.com');

    clearUserEmail();
    expect(getUserEmail()).toBeNull();
  });

  it('isAuthenticated returns true when email is set', () => {
    setUserEmail('test@example.com');
    expect(isAuthenticated()).toBe(true);
  });

  it('isAuthenticated returns false when no email is set', () => {
    expect(isAuthenticated()).toBe(false);
  });

  it('handles special characters in email', () => {
    const email = 'test+tag@example.com';
    setUserEmail(email);
    expect(getUserEmail()).toBe(email);
  });
});
