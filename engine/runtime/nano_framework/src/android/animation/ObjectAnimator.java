package android.animation;

public class ObjectAnimator extends ValueAnimator {
    private Object target;
    private String propertyName;

    public ObjectAnimator() {}

    public static ObjectAnimator ofFloat(Object target, String propertyName, float... values) {
        ObjectAnimator anim = new ObjectAnimator();
        anim.target = target;
        anim.propertyName = propertyName;
        return anim;
    }

    public static ObjectAnimator ofInt(Object target, String propertyName, int... values) {
        ObjectAnimator anim = new ObjectAnimator();
        anim.target = target;
        anim.propertyName = propertyName;
        return anim;
    }

    public static ObjectAnimator ofObject(Object target, String propertyName, TypeEvaluator evaluator, Object... values) {
        ObjectAnimator anim = new ObjectAnimator();
        anim.target = target;
        anim.propertyName = propertyName;
        return anim;
    }

    public Object getTarget() { return target; }
    public void setTarget(Object target) { this.target = target; }
    public String getPropertyName() { return propertyName; }
    public void setPropertyName(String propertyName) { this.propertyName = propertyName; }
}
