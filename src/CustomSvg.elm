module CustomSvg exposing (..)

import Svg exposing (Svg)
import Svg.Attributes as Attributes
import Types exposing (Factory, Transport)


drawTransport : Int -> Int -> String -> Transport -> Svg msg
drawTransport ymin ymax strokeWidth t =
    Svg.line
        [ Attributes.x1 <| String.fromInt t.origin.x
        , Attributes.y1 <| String.fromInt (ymin + ymax - t.origin.y)
        , Attributes.x2 <| String.fromInt t.destination.x
        , Attributes.y2 <| String.fromInt (ymin + ymax - t.destination.y)
        , Attributes.stroke "black"
        , Attributes.strokeWidth strokeWidth
        ]
        []


type alias FactorySvg msg =
    { circle : Svg msg
    , text : Maybe (Svg msg)
    }


drawFactory : Int -> Int -> String -> String -> Factory -> FactorySvg msg
drawFactory ymin ymax r size f =
    if f.active then
        { circle = drawFactoryCircle ymin ymax r f
        , text = Just <| drawFactoryText ymin ymax size f
        }

    else
        { circle = drawFactoryCircle ymin ymax r f
        , text = Just <| drawFactoryText ymin ymax size f
        }


drawFactoryCircle : Int -> Int -> String -> Factory -> Svg msg
drawFactoryCircle ymin ymax r f =
    Svg.circle
        [ Attributes.cx <| String.fromInt f.location.x
        , Attributes.cy <| String.fromInt (ymin + ymax - f.location.y)
        , Attributes.r r
        , Attributes.fill <|
            if f.active then
                if f.profitability > 0 then
                    "blue"

                else
                    "purple"

            else
                "lightgrey"
        ]
        []


drawFactoryText : Int -> Int -> String -> Factory -> Svg msg
drawFactoryText ymin ymax size f =
    Svg.text_
        [ Attributes.x <| String.fromInt f.location.x
        , Attributes.y <| String.fromInt (ymin + ymax - f.location.y + 1300)
        , Attributes.textAnchor "middle"
        , Attributes.alignmentBaseline "middle"
        , Attributes.fontSize size
        ]
        [ Svg.text f.recipe
        ]
