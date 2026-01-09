const USER_EMAIL_COOKIE = 'colony2:user:email';

export function getUserEmail(): string | null {
  if (typeof document === 'undefined') return null;

  const cookies = document.cookie.split(';');
  for (const cookie of cookies) {
    const [key, value] = cookie.trim().split('=');
    if (key === USER_EMAIL_COOKIE) {
      return decodeURIComponent(value);
    }
  }
  return null;
}

export function setUserEmail(email: string): void {
  if (typeof document === 'undefined') return;

  // Set cookie with 1 year expiry
  const expiryDate = new Date();
  expiryDate.setFullYear(expiryDate.getFullYear() + 1);
  document.cookie = `${USER_EMAIL_COOKIE}=${encodeURIComponent(email)}; expires=${expiryDate.toUTCString()}; path=/`;
}

export function clearUserEmail(): void {
  if (typeof document === 'undefined') return;

  // Set cookie with past expiry date to delete it
  document.cookie = `${USER_EMAIL_COOKIE}=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/`;
}

export function isAuthenticated(): boolean {
  return getUserEmail() !== null;
}
