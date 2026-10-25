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

## Not complete / blocked before production

1. **ZIP parity:** this branch modifies the connected GitHub repo `Kadir70-dev/hive` (Next.js 16.4.2). The user-uploaded ZIP was not directly readable in this environment; compare its contents with the repository before merging.
2. **Men-only gatherings:** Meera mentioned women, men and mixed at 11:45 but explicitly specified two categories (mixed and women-only) at 11:53. Confirm whether men-only should be added.
3. **Women-only booking eligibility:** the backend has not been changed in this frontend branch. The category label is not access enforcement; confirm policy and implement/test server-side eligibility before launch.
4. **Visual assets:** concept illustrations are temporary and clearly labelled. Replace or augment them with approved authentic Abu Dhabi photographs sourced/approved by Meera.
5. **Signup analytics:** completed registrations and campaign attribution are not implemented in this frontend-only batch. Agree on privacy disclosure and analytics implementation before paid campaigns.
6. **Bilingual legal pages:** policy content is an English-language draft pending Meera/legal approval and approved Arabic translation.
7. **Partner logos/announcements:** any missing logo data or announcement placement requires backend/content support and Meera's confirmation.
8. **Production readiness:** do not merge/deploy without the repository's security, email, DNS, data migration, backup, staging and legal launch checks.

## QA checklist

- English and Arabic homepage copy and imagery labels; mobile CTA and RTL layout.
- Signup, email verification, pending/approved/declined flow and admin-only access.
- Gathering category filters, reservations, cancellations and FIFO waitlist.
- Member privacy (no attendee list or contact details) and profile deletion.
- No chat, messages, discussions or payment UI.
- Run `npm run lint && npm run typecheck && npm test && npm run build`.
- Review responsive layout on real small screens and verify production CSP before deployment.
