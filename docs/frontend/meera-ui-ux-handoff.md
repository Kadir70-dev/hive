# The Hive Society — frontend UI/UX handoff

Source: Meera's website instructions, 23–24 October 2026. This document describes changes on the feature branch, **not** a production launch approval.

## Implemented in this frontend branch

- Bilingual homepage positioning: Abu Dhabi, men and women, all nationalities, curated experiences, heritage, creativity and sustainability.
- Public gathering access explanation: **mixed-gender** and **women-only**, matching Meera's latest (11:53) two-category wording.
- No member chat: removed the unused public AI conversation section and its event handlers. No member-to-member messaging UI has been added.
- Shorter introductory copy, clear join CTA, and three **conceptual SVG illustrations** of future settings/activities, with explicit English/Arabic labels that they are illustrations rather than past-event photos.
- Responsive signup/login with Arabic and English copy, private registration fields and approval messaging.
- Member gatherings: filter chips, free-event badges, localized categories, booking/waitlist/cancellation UI preserved.
- Admin gathering creation/editing: select one of the two current categories; old unclassified event types are not silently recategorized.
- Gender-neutral Arabic member copy, navigation and profile labels.
- Draft membership terms, privacy copy and SEO/link previews aligned with the expanded audience.
- Regression tests for homepage positioning, removal of chat UI and category classification.
- English/Arabic drafts for Terms and Privacy Policy with responsive RTL layouts; **not legally approved**.
- Arabic RTL-aware admin navigation, partner cards with privacy-safe monogram fallbacks, and optional signup interests.
- Manifest description corrected to welcome all nationalities and genders. Existing H icons retained.
- Server-side women-only booking acknowledgement with lock-safe FIFO reservation flow, cancellation and category-switch guard; no member gender information collected.
- First-party signup campaign attribution: allowlisted UTM fields captured per tab, attached only on signup, persisted against genuinely new accounts, marked as verified only after email confirmation, aggregated in an MFA-protected admin Signups tab. No third-party tracking script, IP, raw referrer or member contact data in reporting. Requires database migration 00005 and staging verification before production use.

## Not complete / blocked before production

1. **Source of truth:** user selected connected `Kadir70-dev/hive` Next.js 16.4.2 + Go as the implementation and `main` merge target, dropping Lovable/TanStack. Uploaded ZIP parity has not been verified.
2. **Men-only gatherings:** Meera mentioned women, men and mixed at 11:45 but explicitly specified two categories (mixed and women-only) at 11:53. Confirm whether men-only should be added.
3. **Women-only booking eligibility:** Go API now requires an explicit per-reservation self-attestation before women-only booking, with backend tests; this is *not* independent identity or gender verification. Meera still needs to approve the exact policy. Admin category changes cannot silently invalidate existing bookings.
4. **Visual assets:** concept illustrations are temporary and clearly labelled. Replace or augment them with approved authentic Abu Dhabi photographs sourced/approved by Meera.
5. **Signup analytics:** first-party code and database migration are implemented in this branch but **not production-verified**. Verify landing UTM → new signup → email verification → admin aggregate report end-to-end, and obtain Meera/privacy approval before paid ads.
6. **Bilingual legal pages:** English and Arabic policy drafts exist; both need Meera/legal approval and professional Arabic review before publication.
7. **Partner logos/announcements:** any missing logo data or announcement placement requires backend/content support and Meera's confirmation.
8. **Production readiness:** main merge is separate from actual website deployment. Do not deploy until security, email, DNS, migration, backup, staging and legal release checks are complete.

## Research-to-code coverage

- Review used the requirements supplied by Meera on 23–24 October 2026. The newest two-category instruction is provisional, not a waiver of the men-only conflict.
- Existing Next.js + Go codebase was **not migrated**. The user instructed us to drop Lovable/TanStack and proceed with this repository.
- Not claimed done: approved authentic visuals, partner logos, confirmed women-only eligibility policy, admin email/DNS, live email deliverability, production E2E/mobile browser testing.

## QA checklist

- English and Arabic homepage copy and imagery labels; mobile CTA and RTL layout.
- Signup, email verification, pending/approved/declined flow and admin-only access.
- Gathering category filters, reservations, cancellations and FIFO waitlist.
- Member privacy (no attendee list or contact details) and profile deletion.
- No chat, messages, discussions or payment UI.
- Run `npm run lint && npm run typecheck && npm test && npm run build`.
- Review responsive layout on real small screens and verify production CSP before deployment.
