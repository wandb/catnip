//
//  CatnipWidgetsLiveActivity.swift
//  CatnipWidgets
//
//  Created by CVP on 10/23/25.
//

import ActivityKit
import WidgetKit
import SwiftUI

struct CatnipWidgetsAttributes: ActivityAttributes {
    public struct ContentState: Codable, Hashable {
        // Dynamic stateful properties about your activity go here!
        var emoji: String
    }

    // Fixed non-changing properties about your activity go here!
    var name: String
}

struct CatnipWidgetsLiveActivity: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: CatnipWidgetsAttributes.self) { context in
            // Lock screen/banner UI goes here
            VStack {
                Text("Hello \(context.state.emoji)")
            }
            .activityBackgroundTint(Color.cyan)
            .activitySystemActionForegroundColor(Color.black)

        } dynamicIsland: { context in
            DynamicIsland {
                // Expanded UI goes here.  Compose the expanded UI through
                // various regions, like leading/trailing/center/bottom
                DynamicIslandExpandedRegion(.leading) {
                    Text("Leading")
                }
                DynamicIslandExpandedRegion(.trailing) {
                    Text("Trailing")
                }
                DynamicIslandExpandedRegion(.bottom) {
                    Text("Bottom \(context.state.emoji)")
                    // more content
                }
            } compactLeading: {
                Text("L")
            } compactTrailing: {
                Text("T \(context.state.emoji)")
            } minimal: {
                Text(context.state.emoji)
            }
            .widgetURL(URL(string: "http://www.apple.com"))
            .keylineTint(Color.red)
        }
    }
}

extension CatnipWidgetsAttributes {
    fileprivate static var preview: CatnipWidgetsAttributes {
        CatnipWidgetsAttributes(name: "World")
    }
}

extension CatnipWidgetsAttributes.ContentState {
    fileprivate static var smiley: CatnipWidgetsAttributes.ContentState {
        CatnipWidgetsAttributes.ContentState(emoji: "😀")
     }
     
     fileprivate static var starEyes: CatnipWidgetsAttributes.ContentState {
         CatnipWidgetsAttributes.ContentState(emoji: "🤩")
     }
}

#Preview("Notification", as: .content, using: CatnipWidgetsAttributes.preview) {
   CatnipWidgetsLiveActivity()
} contentStates: {
    CatnipWidgetsAttributes.ContentState.smiley
    CatnipWidgetsAttributes.ContentState.starEyes
}
