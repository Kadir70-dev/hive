import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MfaGate } from '@/components/screens/MfaGate';
import type { Me } from '@/components/hive/HiveShell';
import { send } from '@/lib/hive-api';

vi.mock('@/lib/hive-api', () => ({ send: vi.fn() }));
vi.mock('qrcode.react', () => ({
  QRCodeSVG: ({ value }: { value: string }) => <svg data-testid="authenticator-qr" data-encoded={value} />,
}));

const member = { email_verified: true, mfa_enabled: false, mfa_verified: false } as Me;
const firstUri = 'otpauth://totp/The%20Hive%20Society:kadir%40example.com?secret=FIRST&issuer=The%20Hive%20Society';
const secondUri = 'otpauth://totp/The%20Hive%20Society:kadir%40example.com?secret=SECOND&issuer=The%20Hive%20Society';

function mount() {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MfaGate me={member}><div>Admin unlocked</div></MfaGate>
    </QueryClientProvider>,
  );
}

beforeEach(() => vi.resetAllMocks());

describe('Admin 2FA QR setup', () => {
  it('encodes the backend otpauth URI in a local SVG QR and keeps the manual option', async () => {
    vi.mocked(send).mockResolvedValueOnce({ secret: 'FIRST', otpauth_uri: firstUri });
    mount();
    expect(screen.queryByTestId('authenticator-qr')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Begin setup' }));

    const qr = await screen.findByTestId('authenticator-qr');
    expect(qr).toHaveAttribute('data-encoded', firstUri);
    expect(screen.getByRole('img', { name: 'Authenticator setup QR code' })).toBeInTheDocument();
    const manual = screen.getByText(/Enter the setup key manually/);
    expect(manual).toBeInTheDocument();
    fireEvent.click(manual);
    expect(screen.getByRole('link', { name: 'open it in the app' })).toHaveAttribute('href', firstUri);
    expect(screen.getByRole('button', { name: 'Generate a new QR code' })).toBeInTheDocument();
    expect(screen.queryByText('Admin unlocked')).not.toBeInTheDocument();
    expect(document.querySelector('img')).toBeNull(); // do not send TOTP secrets to a QR-image service
    expect(send).toHaveBeenCalledWith('/mfa/totp/setup', {});
  });

  it('replaces an exposed pending secret with a newly generated QR', async () => {
    vi.mocked(send)
      .mockResolvedValueOnce({ secret: 'FIRST', otpauth_uri: firstUri })
      .mockResolvedValueOnce({ secret: 'SECOND', otpauth_uri: secondUri });
    mount();
    fireEvent.click(screen.getByRole('button', { name: 'Begin setup' }));
    await screen.findByTestId('authenticator-qr');
    fireEvent.click(screen.getByRole('button', { name: 'Generate a new QR code' }));
    await waitFor(() => expect(screen.getByTestId('authenticator-qr')).toHaveAttribute('data-encoded', secondUri));
    expect(send).toHaveBeenCalledTimes(2);
  });

  it('enables MFA with the code from the scanned authenticator and shows recovery codes', async () => {
    vi.mocked(send).mockResolvedValueOnce({ secret: 'FIRST', otpauth_uri: firstUri })
      .mockResolvedValueOnce({ recovery_codes: ['TEST-RECOVERY-CODE'] });
    mount();
    fireEvent.click(screen.getByRole('button', { name: 'Begin setup' }));
    await screen.findByTestId('authenticator-qr');
    fireEvent.change(screen.getByLabelText('6-digit code from the app'), { target: { value: '123456' } });
    fireEvent.click(screen.getByRole('button', { name: 'Turn on two-factor' }));
    await screen.findByText('Save your recovery codes');
    expect(send).toHaveBeenCalledWith('/mfa/totp/enable', { code: '123456' });
    expect(screen.getByLabelText('Recovery codes')).toHaveTextContent('TEST-RECOVERY-CODE');
  });
});
