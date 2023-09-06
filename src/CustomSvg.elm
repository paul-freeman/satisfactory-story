module CustomSvg exposing (..)

import Svg exposing (Svg)
import Svg.Attributes as Attributes
import Types exposing (Factory, Transport)


drawTransport : Int -> Int -> Transport -> Svg msg
drawTransport ymin ymax t =
    Svg.line
        [ Attributes.x1 <| String.fromInt t.origin.x
        , Attributes.y1 <| String.fromInt (ymin + ymax - t.origin.y)
        , Attributes.x2 <| String.fromInt t.destination.x
        , Attributes.y2 <| String.fromInt (ymin + ymax - t.destination.y)
        , Attributes.stroke "black"
        , Attributes.strokeWidth "100"
        ]
        []


drawFactory : Int -> Int -> Factory -> Svg msg
drawFactory ymin ymax f =
    Svg.g [] <|
        if f.active then
            [ drawFactoryCircle ymin ymax f
            , drawFactoryText ymin ymax f
            ]

        else
            [ drawFactoryCircle ymin ymax f
            ]


drawFactoryCircle : Int -> Int -> Factory -> Svg msg
drawFactoryCircle ymin ymax f =
    Svg.circle
        [ Attributes.cx <| String.fromInt f.location.x
        , Attributes.cy <| String.fromInt (ymin + ymax - f.location.y)
        , Attributes.r "500"
        , Attributes.fill <|
            if f.active then
                "black"

            else
                "lightgrey"
        ]
        []


drawFactoryText : Int -> Int -> Factory -> Svg msg
drawFactoryText ymin ymax f =
    Svg.text_
        [ Attributes.x <| String.fromInt f.location.x
        , Attributes.y <| String.fromInt (ymin + ymax - f.location.y + 1300)
        , Attributes.textAnchor "middle"
        , Attributes.alignmentBaseline "middle"
        , Attributes.fontSize "1000"
        ]
        [ Svg.text f.recipe
        ]
