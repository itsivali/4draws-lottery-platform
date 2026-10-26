import 'package:flutter/semantics.dart';
import 'package:flutter/widgets.dart';

/// Tells screen-reader users about a change without moving focus (results, payment states).
void announce(BuildContext context, String message) {
  SemanticsService.sendAnnouncement(View.of(context), message, Directionality.of(context)).ignore();
}
