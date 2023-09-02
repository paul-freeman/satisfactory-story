module Main exposing (main)

import Browser
import Browser.Dom exposing (Error, Viewport)
import Browser.Events
import Element
import Html exposing (Html)
import Svg exposing (svg)
import Svg.Attributes as Attributes
import Task


type alias Model =
    { svgViewport : Maybe Viewport
    }


initialModel : Model
initialModel =
    { svgViewport = Nothing
    }


type Msg
    = GetSvgViewport (Result Error Viewport)
    | ResizeWindow Int Int


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        GetSvgViewport result ->
            case result of
                Ok viewport ->
                    ( { model | svgViewport = Just viewport }, Cmd.none )

                Err _ ->
                    ( model, Cmd.none )

        ResizeWindow _ _ ->
            ( model, Task.attempt GetSvgViewport Browser.Dom.getViewport )


view : Model -> Html Msg
view model =
    Element.layout [] <|
        Element.column
            [ Element.width Element.fill
            , Element.height Element.fill
            ]
            [ Element.html <| viewSvg model ]


viewSvg : Model -> Html Msg
viewSvg model =
    let
        width =
            model.svgViewport
                |> Maybe.map (.viewport >> .width >> round)
                |> Maybe.withDefault 100

        height =
            model.svgViewport
                |> Maybe.map (.viewport >> .height >> round)
                |> Maybe.withDefault 100
    in
    svg
        [ Attributes.width <| String.fromInt width
        , Attributes.height <| String.fromInt height
        , Attributes.viewBox <| "0 0 " ++ String.fromInt width ++ " " ++ String.fromInt height
        ]
        [ Svg.text "" ]


type alias Flags =
    ()


main : Program Flags Model Msg
main =
    Browser.element
        { init = \_ -> ( initialModel, Task.attempt GetSvgViewport Browser.Dom.getViewport )
        , subscriptions = subscriptions
        , view = view
        , update = update
        }


subscriptions : Model -> Sub Msg
subscriptions _ =
    Browser.Events.onResize ResizeWindow
