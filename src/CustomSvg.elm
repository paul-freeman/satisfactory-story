module CustomSvg exposing (..)

import List exposing (range)
import List.Extra
import Svg exposing (Svg)
import Svg.Attributes as Attributes
import Types exposing (Factory, Resource, Sink, Transport)


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


type alias ResourceSvg msg =
    { circle : Svg msg
    , text : Maybe (Svg msg)
    }


type alias SinkSvg msg =
    { circle : Svg msg
    , text : Maybe (Svg msg)
    }


drawFactory : Int -> Int -> String -> String -> Factory -> FactorySvg msg
drawFactory ymin ymax r size f =
    { circle = drawFactoryCircle ymin ymax r f
    , text = Just <| drawFactoryText ymin ymax size f
    }


drawResource : Int -> Int -> String -> String -> Resource -> ResourceSvg msg
drawResource ymin ymax r size f =
    if f.active then
        { circle = drawResourceCircle ymin ymax r f
        , text = Just <| drawResourceText ymin ymax size f
        }

    else
        { circle = drawResourceCircle ymin ymax r f
        , text = Just <| drawResourceText ymin ymax size f
        }


drawSink : Int -> Int -> String -> String -> Sink -> SinkSvg msg
drawSink ymin ymax r size f =
    { circle = drawSinkCircle ymin ymax r f
    , text = Just <| drawSinkText ymin ymax size f
    }


drawFactoryCircle : Int -> Int -> String -> Factory -> Svg msg
drawFactoryCircle ymin ymax r f =
    Svg.circle
        [ Attributes.cx <| String.fromInt f.location.x
        , Attributes.cy <| String.fromInt (ymin + ymax - f.location.y)
        , Attributes.r r
        , Attributes.fill <|
            if f.profitability > 0 then
                "blue"

            else
                "purple"
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


drawResourceCircle : Int -> Int -> String -> Resource -> Svg msg
drawResourceCircle ymin ymax r f =
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


drawResourceText : Int -> Int -> String -> Resource -> Svg msg
drawResourceText ymin ymax size f =
    Svg.text_
        [ Attributes.x <| String.fromInt f.location.x
        , Attributes.y <| String.fromInt (ymin + ymax - f.location.y + 1300)
        , Attributes.textAnchor "middle"
        , Attributes.alignmentBaseline "middle"
        , Attributes.fontSize size
        ]
        [ Svg.text f.recipe
        ]


drawSinkCircle : Int -> Int -> String -> Sink -> Svg msg
drawSinkCircle ymin ymax r f =
    Svg.circle
        [ Attributes.cx <| String.fromInt f.location.x
        , Attributes.cy <| String.fromInt (ymin + ymax - f.location.y)
        , Attributes.r r
        , Attributes.fill "orange"
        ]
        []


drawSinkText : Int -> Int -> String -> Sink -> Svg msg
drawSinkText ymin ymax size f =
    Svg.text_
        [ Attributes.x <| String.fromInt f.location.x
        , Attributes.y <| String.fromInt (ymin + ymax - f.location.y + 1300)
        , Attributes.textAnchor "middle"
        , Attributes.alignmentBaseline "middle"
        , Attributes.fontSize size
        ]
        [ Svg.text f.label
        ]


mapCornerX : Int
mapCornerX =
    0


mapCornerY : Int
mapCornerY =
    -160300


mapScale : Int
mapScale =
    32000


mapPieces : Svg msg
mapPieces =
    Svg.g [] <| List.Extra.lift2 mapPiece (range 0 4) (range 0 4)


mapPiece : Int -> Int -> Svg msg
mapPiece x y =
    Svg.image
        [ Attributes.xlinkHref <| String.fromInt x ++ "_" ++ String.fromInt y ++ ".png"
        , Attributes.height <| String.fromInt mapScale
        , Attributes.width <| String.fromInt mapScale
        , Attributes.x <| String.fromInt (mapCornerX + x * mapScale)
        , Attributes.y <| String.fromInt (mapCornerY + y * mapScale)
        ]
        []
