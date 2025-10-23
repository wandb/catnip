//
//  CatnipWidgetsBundle.swift
//  CatnipWidgets
//
//  Created by CVP on 10/23/25.
//

import WidgetKit
import SwiftUI

@main
struct CatnipWidgetsBundle: WidgetBundle {
    var body: some Widget {
        if #available(iOS 16.1, *) {
            CodespaceActivityWidget()
        }
    }
}
