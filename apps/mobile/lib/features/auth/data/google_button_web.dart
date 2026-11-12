import 'package:flutter/widgets.dart';
import 'package:google_sign_in_web/web_only.dart' as gsi;

/// The width Google's button is drawn at; whatever holds it must be at least this wide, or its edges are cut off.
const double googleButtonWidth = 300;

/// Google Identity Services renders its own button on the web and will not hand out an ID token any other way.
Widget? googleButton() => gsi.renderButton(
  configuration: gsi.GSIButtonConfiguration(
    theme: gsi.GSIButtonTheme.outline,
    size: gsi.GSIButtonSize.large,
    text: gsi.GSIButtonText.continueWith,
    // a pill, as the app's own buttons
    shape: gsi.GSIButtonShape.pill,
    logoAlignment: gsi.GSIButtonLogoAlignment.left,
    // Google draws it at a fixed width: googleButtonWidth, which the sign-in screen's side column is sized to hold
    minimumWidth: googleButtonWidth,
  ),
);
