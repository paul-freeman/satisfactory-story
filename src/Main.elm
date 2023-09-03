module Main exposing (main)

import Browser
import Browser.Dom exposing (Error, Viewport)
import Browser.Events
import CustomSvg
import Element
import Html exposing (Html)
import Http
import Process
import Svg exposing (svg)
import Svg.Attributes as Attributes
import Task
import Types exposing (State)


type alias Model =
    Result
        String
        { svgViewport : Maybe Viewport
        , state : State
        }


initialModel : Model
initialModel =
    Ok
        { svgViewport = Nothing
        , state = Types.initialState
        }


type Msg
    = GetSvgViewport (Result Error Viewport)
    | ResizeWindow Int Int
    | FetchState (Result Http.Error State) -- Add this for HTTP call
    | PollState -- Add this to loop the HTTP call


update : Msg -> Model -> ( Model, Cmd Msg )
update msg modelRes =
    case modelRes of
        Err _ ->
            ( modelRes, Cmd.none )

        Ok model ->
            case msg of
                GetSvgViewport result ->
                    case result of
                        Ok viewport ->
                            ( Ok { model | svgViewport = Just viewport }, Cmd.none )

                        Err _ ->
                            ( Err "error getting viewport", Cmd.none )

                ResizeWindow _ _ ->
                    ( Ok model, Task.attempt GetSvgViewport Browser.Dom.getViewport )

                FetchState result ->
                    case result of
                        Ok newState ->
                            ( Ok { model | state = newState }, sleepAndPoll )

                        Err _ ->
                            ( Err "error fetching state", sleepAndPoll )

                PollState ->
                    ( Ok model, fetchStateCmd )


view : Model -> Html Msg
view model =
    Element.layout [] <|
        Element.column
            [ Element.width Element.fill
            , Element.height Element.fill
            ]
            [ Element.html <| viewSvg model ]


viewSvg : Model -> Html Msg
viewSvg modelRes =
    case modelRes of
        Err err ->
            Html.text err

        Ok model ->
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
                , Attributes.viewBox <|
                    String.join " "
                        [ String.fromInt model.state.xmin
                        , String.fromInt model.state.ymin
                        , String.fromInt (model.state.xmax - model.state.xmin)
                        , String.fromInt (model.state.ymax - model.state.ymin)
                        ]
                ]
                (List.map (CustomSvg.drawTransport model.state.ymin model.state.ymax) model.state.transports
                    ++ List.map (CustomSvg.drawFactory model.state.ymin model.state.ymax) model.state.factories
                )


type alias Flags =
    ()


main : Program Flags Model Msg
main =
    Browser.element
        { init =
            \_ ->
                ( initialModel
                , Cmd.batch
                    [ Task.attempt GetSvgViewport Browser.Dom.getViewport
                    , fetchStateCmd
                    ]
                )
        , subscriptions = subscriptions
        , view = view
        , update = update
        }


fetchStateCmd : Cmd Msg
fetchStateCmd =
    Http.get
        { url = "http://localhost:28100/json"
        , expect = Http.expectJson FetchState Types.stateDecoder
        }


sleepAndPoll : Cmd Msg
sleepAndPoll =
    Process.sleep 1000
        |> Task.andThen (\_ -> Task.succeed PollState)
        |> Task.perform identity


subscriptions : Model -> Sub Msg
subscriptions _ =
    Browser.Events.onResize ResizeWindow
