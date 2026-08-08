# Wedding RSVP Toast Fields Design

## Goal

Store an RSVP guest's toast participation and companion, and expose both fields through the existing admin RSVP endpoint.

## Chosen Approach

Add `will_give_toast` and `toast_with` to `model.WeddingRSVP`. The existing repository serializes that model to Bolt and the existing admin endpoint returns the same model, so no schema migration, route, or new endpoint is required.

`toast_with` is trimmed and limited to 160 characters. It is cleared when `will_give_toast` is false, so stale companion names cannot remain after a guest changes the answer.

## Verification

An HTTP RSVP test will submit both fields and assert they return in the response. Repository tests continue to cover the persisted model path through the existing JSON-to-Bolt serialization.
