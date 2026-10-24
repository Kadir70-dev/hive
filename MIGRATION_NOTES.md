# Migration direction
Legacy third-party authentication and database code was removed from the maintained application source. This release is a Go/PostgreSQL migration checkpoint. Read IMPLEMENTATION_STATUS.md for blockers and deployment prerequisites. Existing member data is not included and must be exported and migrated securely before turning off the previous service.
