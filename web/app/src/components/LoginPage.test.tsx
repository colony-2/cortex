import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import LoginPage from './LoginPage';
import * as shared from '@colony2/shared';

vi.mock('@colony2/shared', () => ({
  setUserEmail: vi.fn(),
}));

describe('LoginPage', () => {
  it('renders login form', () => {
    const onLogin = vi.fn();
    render(<LoginPage onLogin={onLogin} />);

    expect(screen.getByText('cortex')).toBeInTheDocument();
    expect(screen.getByText('Enter your email to continue')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Email address')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /continue/i })).toBeInTheDocument();
  });

  it('validates email format', async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn();
    render(<LoginPage onLogin={onLogin} />);

    const input = screen.getByPlaceholderText('Email address');
    const button = screen.getByRole('button', { name: /continue/i });

    // Try invalid email
    await user.type(input, 'invalid-email');
    await user.click(button);

    await waitFor(() => {
      expect(screen.getByText('Please enter a valid email')).toBeInTheDocument();
    });

    expect(onLogin).not.toHaveBeenCalled();
  });

  it('requires email to be entered', async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn();
    render(<LoginPage onLogin={onLogin} />);

    const button = screen.getByRole('button', { name: /continue/i });

    // Try submitting without email
    await user.click(button);

    await waitFor(() => {
      expect(screen.getByText('Please enter your email')).toBeInTheDocument();
    });

    expect(onLogin).not.toHaveBeenCalled();
  });

  it('submits valid email and calls onLogin', async () => {
    const user = userEvent.setup();
    const onLogin = vi.fn();
    const setUserEmailMock = vi.mocked(shared.setUserEmail);

    render(<LoginPage onLogin={onLogin} />);

    const input = screen.getByPlaceholderText('Email address');
    const button = screen.getByRole('button', { name: /continue/i });

    // Enter valid email
    await user.type(input, 'test@example.com');
    await user.click(button);

    await waitFor(() => {
      expect(setUserEmailMock).toHaveBeenCalledWith('test@example.com');
      expect(onLogin).toHaveBeenCalled();
    });
  });
});
