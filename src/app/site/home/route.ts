import { renderHome } from '@/lib/cms-home';
import { homeTemplate, publishedDoc } from '@/lib/cms-server';
import { staticHomepageCSP } from '@/lib/security-headers';

export const runtime = 'nodejs';
export const dynamic = 'force-dynamic';

/** The homepage document framed by "/", with the published Studio copy merged in. */
export async function GET(req: Request) {
  const [template, doc] = await Promise.all([homeTemplate(), publishedDoc(req)]);
  const html = renderHome(template, doc);
  return new Response(html, {
    headers: {
      'content-type': 'text/html; charset=utf-8',
      'cache-control': 'no-store',
      // Studio images may be https URLs, so this page also allows https: images.
      'content-security-policy': staticHomepageCSP(html, { httpsImages: true }),
      'x-frame-options': 'SAMEORIGIN',
    },
  });
}
