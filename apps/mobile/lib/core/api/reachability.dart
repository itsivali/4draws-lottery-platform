import 'package:flutter/foundation.dart';

/// Whether the last request reached the server. The API client reports every outcome: a dropped connection, DNS
/// failure or timeout turns it off; any answer from the server, even a refusal, turns it back on. The shell shows a
/// strip while it is off, so a stale screen is never mistaken for a live one.
class Reachability extends ValueNotifier<bool> {
  Reachability() : super(true);

  void report({required bool reached}) {
    if (value != reached) value = reached;
  }
}
